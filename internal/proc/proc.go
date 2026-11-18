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
	phIRQ
)

var validReg = func() map[string]bool {
	m := make(map[string]bool)
	for _, n := range isa.RegisterNames() {
		m[n] = true
	}
	return m
}()

type memOp struct {
	addr int
	wa   int
	hit  bool
	val  int32
}

type Processor struct {
	InstructionMemory []isa.Instruction
	DataMemory        [isa.DataMemSize]byte
	Registers         map[string]int
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

	phase   phase
	ir      isa.Instruction
	wait    int
	total   int
	mem     memOp
	dataEnd int
}

func NewProcessor(code []isa.Instruction, schedule []InputEvent) *Processor {
	return &Processor{
		InstructionMemory: code,
		Registers:         make(map[string]int),
		Cache:             NewCache(CacheLines),
		Dev:               &Device{Schedule: schedule},
	}
}

func (p *Processor) logf(format string, a ...any) {
	if p.Log == nil {
		return
	}
	mode := "main"
	if p.InHandler {
		mode = "irq "
	}
	fmt.Fprintf(p.Log, "TICK %6d PC %4d [%s] %s\n", p.Tick, p.PC, mode, fmt.Sprintf(format, a...))
}

func (p *Processor) fault(format string, a ...any) {
	p.Fault = fmt.Sprintf(format, a...)
	p.logf("FAULT: %s", p.Fault)
	p.Halted = true
}

func (p *Processor) get(name string) int32 {
	if name == "" {
		return 0
	}
	if !validReg[name] {
		p.fault("unknown register %q", name)
		return 0
	}
	return int32(p.Registers[name])
}

func (p *Processor) set(name string, v int32) {
	if name == "" || name == "zero" {
		return
	}
	if !validReg[name] {
		p.fault("unknown register %q", name)
		return
	}
	if name == "sp" && int(v) < p.dataEnd {
		p.fault("stack overflow: sp=%d is inside the data section (ends at %d)", v, p.dataEnd)
		return
	}
	p.Registers[name] = int(v)
}

func (p *Processor) regDump() string {
	var sb strings.Builder
	for _, n := range isa.RegisterNames() {
		if v := p.Registers[n]; n != "zero" && v != 0 {
			fmt.Fprintf(&sb, "%s=%d ", n, v)
		}
	}
	fmt.Fprintf(&sb, "IE=%t", p.IE)
	return sb.String()
}

func (p *Processor) LoadData(data []byte) error {
	if len(data) > len(p.DataMemory) {
		return fmt.Errorf("data section too large: %d bytes", len(data))
	}
	copy(p.DataMemory[:], data)
	p.dataEnd = len(data)
	return nil
}

func (p *Processor) word(addr int) int32 {
	return int32(binary.LittleEndian.Uint32(p.DataMemory[addr : addr+4]))
}

func (p *Processor) setWord(addr int, v int32) {
	binary.LittleEndian.PutUint32(p.DataMemory[addr:addr+4], uint32(v))
}

func (p *Processor) checkAddr(addr, size int) bool {
	if addr < 0 || addr+size > len(p.DataMemory) || addr%size != 0 {
		p.fault("bad data address %d (size %d)", addr, size)
		return false
	}
	return true
}

func (p *Processor) portIn(port int) int32 {
	if port == isa.PortInData {
		v, valid := p.Dev.read()
		if !valid {
			p.logf("in port 0: data register is empty")
		}
		return int32(v)
	}
	p.logf("in from unknown port %d", port)
	return 0
}

func (p *Processor) portOut(port int, v int32) {
	if port == isa.PortOutData {
		p.Dev.Output = append(p.Dev.Output, byte(v))
		return
	}
	p.logf("out to unknown port %d", port)
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
	case phIRQ:
		p.PC = isa.VectorIRQ
		p.phase = phFetch
		p.logf("IRQ vector loaded")
	}

	p.Tick++
	return !p.Halted
}

func (p *Processor) fetch() {
	if p.Pending && p.IE {
		p.EPC = p.PC
		p.Pending = false
		p.IE = false
		p.InHandler = true
		p.phase = phIRQ
		p.logf("ENTER interrupt, epc=%d", p.EPC)
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
	p.PC = next
	p.phase = phFetch
}

func (p *Processor) execute() {
	in := p.ir
	a := p.get(in.Reg1)
	b := p.get(in.Reg2)
	imm := int32(in.Value)
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
	case "slt":
		p.set(in.Reg, b2i(a < b))
	case "slti":
		p.set(in.Reg, b2i(a < imm))
	case "sll":
		p.set(in.Reg, a<<(uint32(b)&31))
	case "srl":
		p.set(in.Reg, int32(uint32(a)>>(uint32(b)&31)))
	case "sra":
		p.set(in.Reg, a>>(uint32(b)&31))
	case "addi":
		p.set(in.Reg, a+imm)
	case "slli":
		p.set(in.Reg, a<<(uint32(imm)&31))
	case "srli":
		p.set(in.Reg, int32(uint32(a)>>(uint32(imm)&31)))
	case "srai":
		p.set(in.Reg, a>>(uint32(imm)&31))
	case "lui":
		p.set(in.Reg, imm<<16)

	case "beq", "bne", "blt", "bge", "bltu", "bgeu", "beqz", "bnez":
		taken := false
		switch in.Opcode {
		case "beq":
			taken = a == b
		case "bne":
			taken = a != b
		case "blt":
			taken = a < b
		case "bge":
			taken = a >= b
		case "bltu":
			taken = uint32(a) < uint32(b)
		case "bgeu":
			taken = uint32(a) >= uint32(b)
		case "beqz":
			taken = a == 0
		case "bnez":
			taken = a != 0
		}
		if taken {
			next = in.Addr
		}
	case "j":
		next = in.Addr
	case "jal":
		p.set(in.Reg, int32(p.PC+4))
		next = in.Addr
	case "jalr":
		target := int(a) + in.Value
		p.set(in.Reg, int32(p.PC+4))
		next = target

	case "lw", "lb", "sw", "sb":
		p.startMem(in, a)
		return

	case "in":
		p.set(in.Reg, p.portIn(in.Value))
	case "out":
		p.portOut(in.Value, p.get(in.Reg))
	case "ei":
		p.IE = true
	case "di":
		p.IE = false
	case "iret":
		next = p.EPC
		p.IE = true
		p.InHandler = false
		p.logf("LEAVE interrupt")
	case "halt":
		p.Halted = true
		p.logf("HALT")
		p.Instructions++
		return
	default:
		p.fault("unknown instruction %q", in.Opcode)
		return
	}

	if p.Halted {
		return
	}
	p.retire(next)
}

func b2i(c bool) int32 {
	if c {
		return 1
	}
	return 0
}

func (p *Processor) startMem(in isa.Instruction, base int32) {
	addr := int(base) + in.Value
	size := 4
	if in.Opcode == "lb" || in.Opcode == "sb" {
		size = 1
	}
	if !p.checkAddr(addr, size) {
		return
	}
	m := memOp{addr: addr, wa: addr &^ 3}

	switch in.Opcode {
	case "lw", "lb":
		v, hit := p.Cache.lookup(m.wa)
		m.hit, m.val = hit, v
		kind := "MISS"
		p.total = MemLatency
		switch {
		case hit:
			kind, p.total = "HIT", CacheLatency
		case !p.Cache.Enabled:
			kind = "NO-CACHE"
		}
		p.logf("MEM    %s addr=%d %s, %d tick(s)", in.Opcode, addr, kind, p.total)
	default:
		m.val = p.get(in.Reg)
		p.total = MemLatency
		p.logf("MEM    %s addr=%d WRITE-THROUGH, %d tick(s)", in.Opcode, addr, p.total)
	}
	p.mem = m
	p.wait = p.total
	p.phase = phMem
}

func (p *Processor) memWait() {
	p.wait--
	p.logf("WAIT   mem %d/%d", p.total-p.wait, p.total)
	if p.wait > 0 {
		return
	}
	p.finishMem()
}

func (p *Processor) finishMem() {
	m := p.mem
	switch p.ir.Opcode {
	case "lw", "lb":
		w := m.val
		if !m.hit {
			w = p.word(m.wa)
			if idx, old := p.Cache.fill(m.wa, w); idx >= 0 {
				if old.valid {
					p.logf("CACHE  fill line %d addr=%d (evicted addr=%d)", idx, m.wa, old.addr)
				} else {
					p.logf("CACHE  fill line %d addr=%d", idx, m.wa)
				}
			}
		}
		if p.ir.Opcode == "lw" {
			p.set(p.ir.Reg, w)
		} else {
			p.set(p.ir.Reg, int32(uint32(w)>>(8*uint(m.addr&3))&0xFF))
		}
	case "sw":
		p.setWord(m.addr, m.val)
		p.Cache.update(m.wa, m.val)
	case "sb":
		sh := 8 * uint(m.addr&3)
		w := uint32(p.word(m.wa))
		w = w&^(0xFF<<sh) | (uint32(m.val)&0xFF)<<sh
		p.setWord(m.wa, int32(w))
		p.Cache.update(m.wa, int32(w))
	}
	if p.Halted {
		return
	}
	p.retire(p.PC + 4)
}
