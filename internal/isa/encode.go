package isa

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const Magic = "AAL1"

type format int

const (
	fmtN format = iota
	fmtR
	fmtI
	fmtB
	fmtJ
)

type opInfo struct {
	code uint32
	f    format
}

var ops = map[string]opInfo{
	"halt": {0, fmtN}, "ei": {1, fmtN}, "di": {2, fmtN}, "iret": {3, fmtN},
	"add": {4, fmtR}, "sub": {5, fmtR}, "mul": {6, fmtR}, "div": {7, fmtR}, "rem": {8, fmtR},
	"slt": {9, fmtR}, "sll": {10, fmtR}, "srl": {11, fmtR}, "sra": {12, fmtR},
	"addi": {13, fmtI}, "slti": {14, fmtI}, "slli": {15, fmtI}, "srli": {16, fmtI}, "srai": {17, fmtI},
	"lui": {18, fmtI}, "lw": {19, fmtI}, "lb": {20, fmtI}, "sw": {21, fmtI}, "sb": {22, fmtI},
	"jalr": {23, fmtI}, "in": {24, fmtI}, "out": {25, fmtI},
	"beq": {26, fmtB}, "bne": {27, fmtB}, "blt": {28, fmtB}, "bge": {29, fmtB},
	"bltu": {30, fmtB}, "bgeu": {31, fmtB}, "beqz": {32, fmtB}, "bnez": {33, fmtB},
	"j": {34, fmtJ}, "jal": {35, fmtJ},
}

var regNames = [32]string{
	"zero", "ra", "sp", "gp", "tp", "t0", "t1", "t2", "s0", "s1", "a0", "a1",
	"a2", "a3", "a4", "a5", "a6", "a7", "s2", "s3", "s4", "s5", "s6", "s7",
	"s8", "s9", "s10", "s11", "t3", "t4", "t5", "t6",
}

var (
	byCode = map[uint32]string{}
	regNum = map[string]uint32{}
)

func init() {
	for name, info := range ops {
		byCode[info.code] = name
	}
	for i, n := range regNames {
		regNum[n] = uint32(i)
	}
}

func RegisterNames() []string { return regNames[:] }

func reg(name string) (uint32, error) {
	if name == "" {
		return 0, nil
	}
	n, ok := regNum[name]
	if !ok {
		return 0, fmt.Errorf("unknown register %q", name)
	}
	return n, nil
}

func fits(v, bits int) bool {
	lim := 1 << (bits - 1)
	return v >= -lim && v < lim
}

func sext(v uint32, bits uint) int {
	return int(int32(v<<(32-bits)) >> (32 - bits))
}

func Encode(in Instruction, pc int) (uint32, error) {
	info, ok := ops[in.Opcode]
	if !ok {
		return 0, fmt.Errorf("unknown opcode %q", in.Opcode)
	}
	w := info.code << 26

	switch info.f {
	case fmtN:
	case fmtR:
		rd, e1 := reg(in.Reg)
		rs1, e2 := reg(in.Reg1)
		rs2, e3 := reg(in.Reg2)
		if e1 != nil || e2 != nil || e3 != nil {
			return 0, fmt.Errorf("%s: bad register", in.Opcode)
		}
		w |= rd<<21 | rs1<<16 | rs2<<11
	case fmtI:
		rd, e1 := reg(in.Reg)
		rs1, e2 := reg(in.Reg1)
		if e1 != nil || e2 != nil {
			return 0, fmt.Errorf("%s: bad register", in.Opcode)
		}
		if !fits(in.Value, 16) {
			return 0, fmt.Errorf("%s: immediate %d does not fit in 16 bits", in.Opcode, in.Value)
		}
		w |= rd<<21 | rs1<<16 | uint32(in.Value)&0xFFFF
	case fmtB:
		rs1, e1 := reg(in.Reg1)
		rs2, e2 := reg(in.Reg2)
		if e1 != nil || e2 != nil {
			return 0, fmt.Errorf("%s: bad register", in.Opcode)
		}
		k := in.Addr - pc
		if !fits(k, 16) {
			return 0, fmt.Errorf("%s: branch too far (%d)", in.Opcode, k)
		}
		w |= rs1<<21 | rs2<<16 | uint32(k)&0xFFFF
	case fmtJ:
		rd, err := reg(in.Reg)
		if err != nil {
			return 0, err
		}
		k := in.Addr - pc
		if !fits(k, 21) {
			return 0, fmt.Errorf("%s: jump too far (%d)", in.Opcode, k)
		}
		w |= rd<<21 | uint32(k)&0x1FFFFF
	}
	return w, nil
}

func Decode(w uint32, pc int) (Instruction, error) {
	name, ok := byCode[w>>26]
	if !ok {
		return Instruction{}, fmt.Errorf("unknown opcode %d at pc=%d", w>>26, pc)
	}
	in := Instruction{Opcode: name}
	switch ops[name].f {
	case fmtR:
		in.Reg, in.Reg1, in.Reg2 = regNames[w>>21&31], regNames[w>>16&31], regNames[w>>11&31]
	case fmtI:
		in.Reg, in.Reg1 = regNames[w>>21&31], regNames[w>>16&31]
		in.Value = sext(w&0xFFFF, 16)
	case fmtB:
		in.Reg1, in.Reg2 = regNames[w>>21&31], regNames[w>>16&31]
		in.Addr = pc + sext(w&0xFFFF, 16)
	case fmtJ:
		in.Reg = regNames[w>>21&31]
		in.Addr = pc + sext(w&0x1FFFFF, 21)
	}
	return in, nil
}

func (in Instruction) String() string {
	info, ok := ops[in.Opcode]
	if !ok {
		return in.Opcode
	}
	switch info.f {
	case fmtN:
		return in.Opcode
	case fmtR:
		return fmt.Sprintf("%s %s, %s, %s", in.Opcode, in.Reg, in.Reg1, in.Reg2)
	case fmtI:
		switch in.Opcode {
		case "lw", "lb", "sw", "sb":
			return fmt.Sprintf("%s %s, %d(%s)", in.Opcode, in.Reg, in.Value, in.Reg1)
		case "in", "out", "lui":
			return fmt.Sprintf("%s %s, %d", in.Opcode, in.Reg, in.Value)
		}
		return fmt.Sprintf("%s %s, %s, %d", in.Opcode, in.Reg, in.Reg1, in.Value)
	case fmtB:
		if in.Opcode == "beqz" || in.Opcode == "bnez" {
			return fmt.Sprintf("%s %s, 0x%04X", in.Opcode, in.Reg1, in.Addr)
		}
		return fmt.Sprintf("%s %s, %s, 0x%04X", in.Opcode, in.Reg1, in.Reg2, in.Addr)
	default:
		if in.Opcode == "j" {
			return fmt.Sprintf("j 0x%04X", in.Addr)
		}
		return fmt.Sprintf("jal %s, 0x%04X", in.Reg, in.Addr)
	}
}

func Assemble(code []Instruction, data []byte) ([]byte, string, error) {
	bin := make([]byte, 12)
	copy(bin, Magic)
	binary.LittleEndian.PutUint32(bin[4:], uint32(len(code)*4))
	binary.LittleEndian.PutUint32(bin[8:], uint32(len(data)))

	var sb strings.Builder
	sb.WriteString(".text\n")
	for i, in := range code {
		pc := i * 4
		w, err := Encode(in, pc)
		if err != nil {
			return nil, "", fmt.Errorf("at 0x%04X: %w", pc, err)
		}
		bin = binary.LittleEndian.AppendUint32(bin, w)
		d, _ := Decode(w, pc)
		fmt.Fprintf(&sb, "%04X - %08X - %s\n", pc, w, d)
	}
	sb.WriteString(".data\n")
	for i := 0; i < len(data); i += 16 {
		end := min(i+16, len(data))
		fmt.Fprintf(&sb, "%04X - % X\n", i, data[i:end])
	}
	return append(bin, data...), sb.String(), nil
}

func Load(bin []byte) ([]Instruction, []byte, error) {
	if len(bin) < 12 || string(bin[:4]) != Magic {
		return nil, nil, fmt.Errorf("not a valid program file")
	}
	cs := int(binary.LittleEndian.Uint32(bin[4:]))
	ds := int(binary.LittleEndian.Uint32(bin[8:]))
	if cs%4 != 0 || len(bin) != 12+cs+ds {
		return nil, nil, fmt.Errorf("corrupted program file")
	}
	code := make([]Instruction, cs/4)
	for i := range code {
		w := binary.LittleEndian.Uint32(bin[12+i*4:])
		in, err := Decode(w, i*4)
		if err != nil {
			return nil, nil, err
		}
		code[i] = in
	}
	return code, bin[12+cs:], nil
}
