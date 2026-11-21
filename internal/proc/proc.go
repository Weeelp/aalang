package proc

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/Weeelp/aalang/internal/isa"
)

const (
	CacheLatency = 1
	MemLatency   = 10
)

type phase int

const (
	phFetch phase = iota
	phExec
	phMem
)

type memOp struct {
	addr int
	hit  bool
	val  int32
}

type Processor struct {
	InstructionMemory []isa.Instruction
	DataMemory        [isa.DataMemSize]byte
	Registers         map[string]int32
	PC                int
	Tick              int
	Instructions      int
	Halted            bool
	Fault             string

	Cache *Cache
	Dev   *Device

	IE        bool
	Pending   bool
	InHandler bool
	EPC       int

	Log io.Writer

	// CU
	phase   phase
	ir      isa.Instruction
	mem     memOp
	wait    int
	total   int
	dataEnd int
}

func NewProcessor(code []isa.Instruction, schedule []InputEvent) *Processor {
	return &Processor{
		InstructionMemory: code,
		Registers:         map[string]int32{},
		Cache:             NewCache(CacheLines),
		Dev:               &Device{Schedule: schedule},
	}
}

func (p *Processor) LoadData(data []byte) error {
	if len(data) > len(p.DataMemory) {
		return fmt.Errorf("data section too large: %d bytes", len(data))
	}
	copy(p.DataMemory[:], data)
	p.dataEnd = len(data)
	return nil
}

func (p *Processor) logf(format string, a ...any) {
	if p.Log == nil {
		return
	}
	mode := "main"
	if p.InHandler {
		mode = "irq "
	}
	fmt.Fprintf(p.Log, "TICK %6d | PC %4d | [%s] %s\n", p.Tick, p.PC, mode, fmt.Sprintf(format, a...))
}

func (p *Processor) fault(format string, a ...any) {
	p.Fault = fmt.Sprintf(format, a...)
	p.logf("FAULT: %s", p.Fault)
	p.Halted = true
}

func (p *Processor) set(r string, v int32) {
	if r == "" || r == "zero" {
		return
	}
	if r == "sp" && int(v) < p.dataEnd {
		p.fault("stack overflow: sp=%d is inside the data section (ends at %d)", v, p.dataEnd)
		return
	}
	p.Registers[r] = v
}

func (p *Processor) regDump() string {
	var sb strings.Builder
	for _, n := range isa.RegisterNames() {
		if v := p.Registers[n]; v != 0 {
			fmt.Fprintf(&sb, "%s=%d ", n, v)
		}
	}
	fmt.Fprintf(&sb, "IE=%t", p.IE)
	return sb.String()
}

func (p *Processor) word(addr int) int32 {
	return int32(binary.LittleEndian.Uint32(p.DataMemory[addr:]))
}

func (p *Processor) setWord(addr int, v int32) {
	binary.LittleEndian.PutUint32(p.DataMemory[addr:], uint32(v))
}

func (p *Processor) Cycle() bool {
	if p.Halted {
		return false
	}
	if raised, lost := p.Dev.poll(p.Tick); raised {
		p.Pending = true
		p.logf("IRQ raised (data=%q)", p.Dev.DataReg)
		if lost > 0 {
			p.logf("OVERRUN: %d char(s) lost", lost)
		}
	}
	switch p.phase {
	case phFetch:
		p.fetch()
	case phExec:
		p.execute()
	case phMem:
		p.memWait()
	}
	p.Tick++
	return !p.Halted
}

func (p *Processor) fetch() {
	if p.Pending && p.IE {
		p.logf("ENTER interrupt, epc=%d", p.PC)
		p.EPC, p.PC = p.PC, isa.VectorIRQ
		p.Pending, p.IE, p.InHandler = false, false, true
		return
	}
	idx := p.PC / 4
	if p.PC < 0 || p.PC%4 != 0 || idx >= len(p.InstructionMemory) {
		p.fault("PC out of range: %d", p.PC)
		return
	}
	p.ir = p.InstructionMemory[idx]
	p.phase = phExec
	p.logf("FETCH  %s", p.ir)
}

func (p *Processor) retire(next int) {
	p.Instructions++
	p.logf("RETIRE %s | %s", p.ir, p.regDump())
	p.PC, p.phase = next, phFetch
}

func taken(op string, a, b int32) bool {
	switch op {
	case "beq":
		return a == b
	case "bne":
		return a != b
	case "blt":
		return a < b
	case "bge":
		return a >= b
	case "bltu":
		return uint32(a) < uint32(b)
	}
	return a == 0 // beqz
}

func (p *Processor) execute() {
	in := p.ir
	a, b, imm := p.Registers[in.Reg1], p.Registers[in.Reg2], int32(in.Value)
	next := p.PC + 4

	switch in.Opcode {
	case "add":
		p.set(in.Reg, a+b)
	case "sub":
		p.set(in.Reg, a-b)
	case "mul":
		p.set(in.Reg, a*b)
	case "div":
		if b == 0 {
			p.set(in.Reg, -1)
		} else {
			p.set(in.Reg, a/b)
		}
	case "rem":
		if b == 0 {
			p.set(in.Reg, a)
		} else {
			p.set(in.Reg, a%b)
		}
	case "addi":
		p.set(in.Reg, a+imm)
	case "lui":
		p.set(in.Reg, imm<<16)

	case "beq", "bne", "blt", "bge", "bltu", "beqz":
		if taken(in.Opcode, a, b) {
			next = in.Addr
		}
	case "j":
		next = in.Addr
	case "jal":
		p.set(in.Reg, int32(p.PC+4))
		next = in.Addr
	case "jalr":
		next = int(a) + in.Value
		p.set(in.Reg, int32(p.PC+4))

	case "lw", "lb", "sw", "sb":
		p.startMem(in, a)
		return

	case "in":
		v, valid := p.Dev.read()
		if !valid {
			p.logf("in port %d: data register is empty", in.Value)
		}
		p.set(in.Reg, int32(v))
	case "out":
		p.Dev.Output = append(p.Dev.Output, byte(p.Registers[in.Reg]))
	case "ei":
		p.IE = true
	case "di":
		p.IE = false
	case "iret":
		next = p.EPC
		p.IE, p.InHandler = true, false
		p.logf("LEAVE interrupt")
	case "halt":
		p.Halted = true
		p.Instructions++
		p.logf("HALT")
		return
	default:
		p.fault("unknown instruction %q", in.Opcode)
	}
	if !p.Halted {
		p.retire(next)
	}
}

func (p *Processor) startMem(in isa.Instruction, base int32) {
	addr, size := int(base)+in.Value, 4
	if in.Opcode == "lb" || in.Opcode == "sb" {
		size = 1
	}
	if addr < 0 || addr+size > len(p.DataMemory) || addr%size != 0 {
		p.fault("bad data address %d (size %d)", addr, size)
		return
	}

	m := memOp{addr: addr}
	p.total = MemLatency
	kind := "WRITE-THROUGH"
	if in.Opcode[0] == 'l' {
		m.val, m.hit = p.Cache.lookup(addr &^ 3)
		switch {
		case m.hit:
			kind, p.total = "HIT", CacheLatency
		case p.Cache.Enabled:
			kind = "MISS"
		default:
			kind = "NO-CACHE"
		}
	} else {
		m.val = p.Registers[in.Reg]
	}
	p.logf("MEM    %s addr=%d %s, %d tick(s)", in.Opcode, addr, kind, p.total)
	p.mem, p.wait, p.phase = m, p.total, phMem
}

func (p *Processor) memWait() {
	p.wait--
	p.logf("WAIT   mem %d/%d", p.total-p.wait, p.total)
	if p.wait == 0 {
		p.finishMem()
	}
}

func (p *Processor) finishMem() {
	m, wa := p.mem, p.mem.addr&^3
	sh := 8 * uint(m.addr&3)
	switch p.ir.Opcode {
	case "lw", "lb":
		w := m.val
		if !m.hit {
			w = p.word(wa)
			if idx, old := p.Cache.fill(wa, w); idx >= 0 {
				p.logf("CACHE  fill line %d addr=%d (evicted valid=%t addr=%d)", idx, wa, old.valid, old.addr)
			}
		}
		if p.ir.Opcode == "lb" {
			w = int32(uint32(w) >> sh & 0xFF)
		}
		p.set(p.ir.Reg, w)
	case "sw":
		p.setWord(wa, m.val)
		p.Cache.update(wa, m.val)
	case "sb":
		w := uint32(p.word(wa))&^(0xFF<<sh) | (uint32(m.val)&0xFF)<<sh
		p.setWord(wa, int32(w))
		p.Cache.update(wa, int32(w))
	}
	p.retire(p.PC + 4)
}
