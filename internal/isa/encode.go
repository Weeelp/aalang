package isa

import (
	"encoding/binary"
	"errors"
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

var opList = []struct {
	name string
	f    format
}{
	{"halt", fmtN}, {"ei", fmtN}, {"di", fmtN}, {"iret", fmtN},
	{"add", fmtR}, {"sub", fmtR}, {"mul", fmtR}, {"div", fmtR},
	{"rem", fmtR}, {"addi", fmtI}, {"lui", fmtI}, {"lw", fmtI},
	{"lb", fmtI}, {"sw", fmtI}, {"sb", fmtI}, {"jalr", fmtI},
	{"in", fmtI}, {"out", fmtI}, {"beq", fmtB}, {"bne", fmtB},
	{"blt", fmtB}, {"bge", fmtB}, {"bltu", fmtB}, {"beqz", fmtB},
	{"j", fmtJ}, {"jal", fmtJ},
}

var regNames = [32]string{
	"zero", "ra", "sp", "gp", "tp", "t0", "t1", "t2", "s0", "s1", "a0", "a1",
	"a2", "a3", "a4", "a5", "a6", "a7", "s2", "s3", "s4", "s5", "s6", "s7",
	"s8", "s9", "s10", "s11", "t3", "t4", "t5", "t6",
}

var (
	opCode = map[string]uint32{}
	regNum = map[string]uint32{}
)

func init() {
	for i, o := range opList {
		opCode[o.name] = uint32(i)
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

func fits(v, bits int) bool { return v >= -(1<<(bits-1)) && v < 1<<(bits-1) }

func sext(v uint32, bits uint) int { return int(int32(v<<(32-bits)) >> (32 - bits)) }

func Encode(in Instruction, pc int) (uint32, error) {
	code, ok := opCode[in.Opcode]
	if !ok {
		return 0, fmt.Errorf("unknown opcode %q", in.Opcode)
	}
	rd, e1 := reg(in.Reg)
	rs1, e2 := reg(in.Reg1)
	rs2, e3 := reg(in.Reg2)
	if err := errors.Join(e1, e2, e3); err != nil {
		return 0, err
	}

	w, k := code<<26, in.Addr-pc
	switch opList[code].f {
	case fmtR:
		w |= rd<<21 | rs1<<16 | rs2<<11
	case fmtI:
		if !fits(in.Value, 16) {
			return 0, fmt.Errorf("%s: immediate %d does not fit in 16 bits", in.Opcode, in.Value)
		}
		w |= rd<<21 | rs1<<16 | uint32(in.Value)&0xFFFF
	case fmtB:
		if !fits(k, 16) {
			return 0, fmt.Errorf("%s: branch too far (%d)", in.Opcode, k)
		}
		w |= rs1<<21 | rs2<<16 | uint32(k)&0xFFFF
	case fmtJ:
		if !fits(k, 21) {
			return 0, fmt.Errorf("%s: jump too far (%d)", in.Opcode, k)
		}
		w |= rd<<21 | uint32(k)&0x1FFFFF
	}
	return w, nil
}

func Decode(w uint32, pc int) (Instruction, error) {
	code := int(w >> 26)
	if code >= len(opList) {
		return Instruction{}, fmt.Errorf("unknown opcode %d at pc=%d", code, pc)
	}
	f, a, b, c := opList[code].f, regNames[w>>21&31], regNames[w>>16&31], regNames[w>>11&31]
	in := Instruction{Opcode: opList[code].name}
	switch f {
	case fmtR:
		in.Reg, in.Reg1, in.Reg2 = a, b, c
	case fmtI:
		in.Reg, in.Reg1, in.Value = a, b, sext(w&0xFFFF, 16)
	case fmtB:
		in.Reg1, in.Reg2, in.Addr = a, b, pc+sext(w&0xFFFF, 16)
	case fmtJ:
		in.Reg, in.Addr = a, pc+sext(w&0x1FFFFF, 21)
	}
	return in, nil
}

func (in Instruction) String() string {
	code, ok := opCode[in.Opcode]
	if !ok {
		return in.Opcode
	}
	switch in.Opcode {
	case "lw", "lb", "sw", "sb":
		return fmt.Sprintf("%s %s, %d(%s)", in.Opcode, in.Reg, in.Value, in.Reg1)
	case "in", "out", "lui":
		return fmt.Sprintf("%s %s, %d", in.Opcode, in.Reg, in.Value)
	case "beqz":
		return fmt.Sprintf("beqz %s, 0x%04X", in.Reg1, in.Addr)
	case "j":
		return fmt.Sprintf("j 0x%04X", in.Addr)
	case "jal":
		return fmt.Sprintf("jal %s, 0x%04X", in.Reg, in.Addr)
	}
	switch opList[code].f {
	case fmtR:
		return fmt.Sprintf("%s %s, %s, %s", in.Opcode, in.Reg, in.Reg1, in.Reg2)
	case fmtI:
		return fmt.Sprintf("%s %s, %s, %d", in.Opcode, in.Reg, in.Reg1, in.Value)
	case fmtB:
		return fmt.Sprintf("%s %s, %s, 0x%04X", in.Opcode, in.Reg1, in.Reg2, in.Addr)
	}
	return in.Opcode
}

func Assemble(code []Instruction, data []byte) ([]byte, string, error) {
	bin := make([]byte, 12)
	copy(bin, Magic)
	binary.LittleEndian.PutUint32(bin[4:], uint32(len(code)*4))
	binary.LittleEndian.PutUint32(bin[8:], uint32(len(data)))

	var sb strings.Builder
	sb.WriteString(".text\n")
	for i, in := range code {
		w, err := Encode(in, i*4)
		if err != nil {
			return nil, "", fmt.Errorf("at 0x%04X: %w", i*4, err)
		}
		bin = binary.LittleEndian.AppendUint32(bin, w)
		d, _ := Decode(w, i*4)
		fmt.Fprintf(&sb, "%04X - %08X - %s\n", i*4, w, d)
	}
	sb.WriteString(".data\n")
	for i := 0; i < len(data); i += 16 {
		fmt.Fprintf(&sb, "%04X - % X\n", i, data[i:min(i+16, len(data))])
	}
	return append(bin, data...), sb.String(), nil
}

func Load(bin []byte) ([]Instruction, []byte, error) {
	if len(bin) < 12 || string(bin[:4]) != Magic {
		return nil, nil, errors.New("not a valid program file")
	}
	cs, ds := int(binary.LittleEndian.Uint32(bin[4:])), int(binary.LittleEndian.Uint32(bin[8:]))
	if cs%4 != 0 || len(bin) != 12+cs+ds {
		return nil, nil, errors.New("corrupted program file")
	}
	code := make([]Instruction, cs/4)
	for i := range code {
		in, err := Decode(binary.LittleEndian.Uint32(bin[12+i*4:]), i*4)
		if err != nil {
			return nil, nil, err
		}
		code[i] = in
	}
	return code, bin[12+cs:], nil
}
