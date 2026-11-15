package proc

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/Weeelp/aalang/internal/isa"
)

type CacheLine struct {
	Valid bool
	Addr  int
	Value int
}

type Processor struct {
	InstructionMemory []isa.Instruction
	DataMemory        [isa.DataMemSize]byte
	Registers         map[string]int
	PC                int
	Tick              int

	Cache          [4]CacheLine
	CacheHitCount  int
	CacheMissCount int

	Dev       *Device
	IE        bool
	Pending   bool
	InHandler bool
	EPC       int
	Log       io.Writer
}

func NewProcessor(code []isa.Instruction, schedule []InputEvent) *Processor {
	regs := make(map[string]int)

	regs["zero"] = 0

	regs["ra"] = 0
	regs["sp"] = 0
	regs["gp"] = 0
	regs["tp"] = 0

	for i := 0; i <= 7; i++ {
		regs[fmt.Sprintf("a%d", i)] = 0
	}

	for i := 0; i <= 6; i++ {
		regs[fmt.Sprintf("t%d", i)] = 0
	}

	regs["s0"] = 0

	for i := 1; i <= 11; i++ {
		regs[fmt.Sprintf("s%d", i)] = 0
	}

	return &Processor{
		InstructionMemory: code,
		Registers:         regs,
		PC:                0,
		Tick:              0,
		Dev:               &Device{Schedule: schedule},
	}
}

// TODO
func (p *Processor) logf(format string, a ...any) {
	if p.Log == nil {
		return
	}
	fmt.Fprintf(p.Log, "TICK %6d PC %3d | "+format+"\n", append([]any{p.Tick, p.PC}, a...)...)
}

func (p *Processor) Step() bool {
	if raised, lost := p.Dev.poll(p.Tick); raised {
		p.Pending = true
		p.logf("IRQ raised (data=%q)", p.Dev.DataReg)
		if lost > 0 {
			p.logf("OVERRUN: %d char(s) lost", lost)
		}
	}

	if p.Pending && p.IE {
		p.Pending = false
		p.EPC = p.PC
		p.IE = false
		p.InHandler = true
		p.logf("ENTER interrupt, epc=%d", p.EPC)
		p.PC = isa.VectorIRQ
		p.Tick += 2
	}

	idx := p.PC / 4
	if p.PC >= 0 && idx < len(p.InstructionMemory) {
		mode := "main"
		if p.InHandler {
			mode = "irq "
		}
		p.logf("[%s] %s", mode, p.InstructionMemory[idx])
	} else {
		p.logf("PC out of range, stopping")
	}
	return p.RunStep()
}

func (p *Processor) portIn(port int) int {
	if port == isa.PortInData {
		v, valid := p.Dev.read()
		if !valid {
			p.logf("in port 0: data register is empty")
		}
		return v
	}
	p.logf("in from unknown port %d", port)
	return 0
}

func (p *Processor) portOut(port, v int) {
	if port == isa.PortOutData {
		p.Dev.Output = append(p.Dev.Output, byte(v))
		return
	}
	p.logf("out to unknown port %d", port)
}

func (p *Processor) readData(addr int) int {
	for i := 0; i < len(p.Cache); i++ {
		if p.Cache[i].Valid && p.Cache[i].Addr == addr {
			p.CacheHitCount++
			p.Tick += 1
			return p.Cache[i].Value
		}
	}

	p.CacheMissCount++
	p.Tick += 10
	value := p.word(addr)

	victimIdx := p.CacheMissCount % 4
	p.Cache[victimIdx] = CacheLine{Valid: true, Addr: addr, Value: value}

	return value
}

func (p *Processor) writeData(addr int, value int) {
	p.setWord(addr, value)
	p.Tick += 10

	updated := false
	for i := 0; i < len(p.Cache); i++ {
		if p.Cache[i].Valid && p.Cache[i].Addr == addr {
			p.Cache[i].Value = value
			updated = true
			break
		}
	}

	if !updated {
		victimIdx := p.Tick % 4
		p.Cache[victimIdx] = CacheLine{Valid: true, Addr: addr, Value: value}
	}
}

func (p *Processor) LoadData(data []byte) error {
	if len(data) > len(p.DataMemory) {
		return fmt.Errorf("data section too large: %d bytes", len(data))
	}
	copy(p.DataMemory[:], data)
	return nil
}

func (p *Processor) word(addr int) int {
	return int(int32(binary.LittleEndian.Uint32(p.DataMemory[addr : addr+4])))
}

func (p *Processor) setWord(addr, v int) {
	binary.LittleEndian.PutUint32(p.DataMemory[addr:addr+4], uint32(int32(v)))
}

func (p *Processor) checkAddr(addr, size int) bool {
	if addr < 0 || addr+size > len(p.DataMemory) || addr%size != 0 {
		p.logf("FAULT: bad data address %d (size %d)", addr, size)
		return false
	}
	return true
}

func (p *Processor) RunStep() bool {
	if p.PC < 0 || p.PC/4 >= len(p.InstructionMemory) {
		return false
	}

	p.Registers["zero"] = 0

	instr := p.InstructionMemory[p.PC/4]

	switch instr.Opcode {
	case "addi":
		p.Registers[instr.Reg] = p.Registers[instr.Reg1] + instr.Value
		p.Tick++
		p.PC += 4

	case "lw":
		addr := p.Registers[instr.Reg1] + instr.Value
		if !p.checkAddr(addr, 4) {
			return false
		}
		p.Registers[instr.Reg] = p.readData(addr)
		p.PC += 4

	case "sw":
		addr := p.Registers[instr.Reg1] + instr.Value
		if !p.checkAddr(addr, 4) {
			return false
		}
		p.writeData(addr, p.Registers[instr.Reg])
		p.PC += 4

	case "lb":
		addr := p.Registers[instr.Reg1] + instr.Value
		if !p.checkAddr(addr, 1) {
			return false
		}
		w := uint32(p.readData(addr &^ 3))
		p.Registers[instr.Reg] = int(w >> (8 * uint(addr&3)) & 0xFF)
		p.PC += 4

	case "sb":
		addr := p.Registers[instr.Reg1] + instr.Value
		if !p.checkAddr(addr, 1) {
			return false
		}
		sh := 8 * uint(addr&3)
		w := uint32(p.readData(addr &^ 3))
		w = w&^(0xFF<<sh) | uint32(p.Registers[instr.Reg]&0xFF)<<sh
		p.writeData(addr&^3, int(int32(w)))
		p.PC += 4

	case "lui":
		p.Registers[instr.Reg] = int(int32(instr.Value) << 16)
		p.Tick++
		p.PC += 4

	case "add":
		p.Registers[instr.Reg] = p.Registers[instr.Reg1] + p.Registers[instr.Reg2]
		p.Tick++
		p.PC += 4

	case "sub":
		p.Registers[instr.Reg] = p.Registers[instr.Reg1] - p.Registers[instr.Reg2]
		p.Tick++
		p.PC += 4

	case "mul":
		p.Registers[instr.Reg] = p.Registers[instr.Reg1] * p.Registers[instr.Reg2]
		p.Tick++
		p.PC += 4

	case "div":
		if p.Registers[instr.Reg2] == 0 {
			p.Registers[instr.Reg] = -1
		} else {
			p.Registers[instr.Reg] = p.Registers[instr.Reg1] / p.Registers[instr.Reg2]
		}
		p.Tick++
		p.PC += 4

	case "rem":
		if p.Registers[instr.Reg2] == 0 {
			p.Registers[instr.Reg] = p.Registers[instr.Reg1]
		} else {
			p.Registers[instr.Reg] = p.Registers[instr.Reg1] % p.Registers[instr.Reg2]
		}
		p.Tick++
		p.PC += 4

	case "slt":
		if p.Registers[instr.Reg1] < p.Registers[instr.Reg2] {
			p.Registers[instr.Reg] = 1
		} else {
			p.Registers[instr.Reg] = 0
		}
		p.Tick++
		p.PC += 4

	case "slti":
		if p.Registers[instr.Reg1] < instr.Value {
			p.Registers[instr.Reg] = 1
		} else {
			p.Registers[instr.Reg] = 0
		}
		p.Tick++
		p.PC += 4

	case "j":
		p.PC = instr.Addr
		p.Tick++

	case "beq", "bne", "blt", "bge", "bltu", "bgeu", "beqz", "bnez":
		a := int32(p.Registers[instr.Reg1])
		b := int32(p.Registers[instr.Reg2])
		taken := false
		switch instr.Opcode {
		case "beq", "beqz":
			taken = a == b
		case "bne", "bnez":
			taken = a != b
		case "blt":
			taken = a < b
		case "bge":
			taken = a >= b
		case "bltu":
			taken = uint32(a) < uint32(b)
		case "bgeu":
			taken = uint32(a) >= uint32(b)
		}
		if taken {
			p.PC = instr.Addr
		} else {
			p.PC += 4
		}
		p.Tick++

	case "jal":
		p.Registers[instr.Reg] = p.PC + 4
		p.PC = instr.Addr
		p.Tick++

	case "jalr":
		ret := p.PC + 4
		p.PC = p.Registers[instr.Reg1] + instr.Value
		p.Registers[instr.Reg] = ret
		p.Tick++

	case "halt":
		return false

	case "in":
		p.Registers[instr.Reg] = p.portIn(instr.Value)
		p.Tick++
		p.PC += 4

	case "out":
		p.portOut(instr.Value, p.Registers[instr.Reg])
		p.Tick++
		p.PC += 4

	case "ei":
		p.IE = true
		p.Tick++
		p.PC += 4

	case "di":
		p.IE = false
		p.Tick++
		p.PC += 4

	case "iret":
		p.PC = p.EPC
		p.IE = true
		p.InHandler = false
		p.Tick += 2
		p.logf("LEAVE interrupt")

	default:
		fmt.Printf("Критическая ошибка: неизвестная инструкция %s\n", instr.Opcode)
		return false
	}

	p.Registers["zero"] = 0

	return true
}
