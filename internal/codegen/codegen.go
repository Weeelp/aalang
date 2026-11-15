package codegen

import (
	"fmt"

	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/parser"
)

var binOps = map[string]string{
	"+": "add", "-": "sub", "*": "mul", "/": "div", "%": "rem",
}

type item struct {
	ins    isa.Instruction
	label  string
	target string
}

type Compiler struct {
	code     []item
	symbols  map[string]int
	data     []byte
	strs     map[string]int
	labelCnt int
	freeRegs []string
}

func New() *Compiler {
	return &Compiler{
		symbols:  make(map[string]int),
		strs:     make(map[string]int),
		freeRegs: []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6"},
	}
}

func (c *Compiler) emit(i isa.Instruction) {
	c.code = append(c.code, item{ins: i})
}

func (c *Compiler) emitLabel(name string) {
	c.code = append(c.code, item{label: name})
}

func (c *Compiler) emitJump(i isa.Instruction, target string) {
	c.code = append(c.code, item{ins: i, target: target})
}

func (c *Compiler) newLabel(prefix string) string {
	c.labelCnt++
	return fmt.Sprintf("%s_%d", prefix, c.labelCnt)
}

func (c *Compiler) allocReg() (string, error) {
	if len(c.freeRegs) == 0 {
		return "", fmt.Errorf("Expression too nested: out of temporary registers")
	}
	r := c.freeRegs[0]
	c.freeRegs = c.freeRegs[1:]
	return r, nil
}

func (c *Compiler) freeReg(r string) {
	c.freeRegs = append([]string{r}, c.freeRegs...)
}

func (c *Compiler) alloc(b []byte) (int, error) {
	for len(c.data)%4 != 0 {
		c.data = append(c.data, 0)
	}
	addr := len(c.data)
	if addr+len(b) > isa.DataMemSize {
		return 0, fmt.Errorf("Data memory has run out")
	}
	c.data = append(c.data, b...)
	return addr, nil
}

func (c *Compiler) declare(name string) (int, error) {
	if addr, ok := c.symbols[name]; ok {
		return addr, nil
	}
	addr, err := c.alloc(make([]byte, 4))
	if err != nil {
		return 0, err
	}
	c.symbols[name] = addr
	return addr, nil
}

func (c *Compiler) internString(s string) (int, error) {
	if addr, ok := c.strs[s]; ok {
		return addr, nil
	}
	addr, err := c.alloc(append([]byte(s), 0))
	if err != nil {
		return 0, err
	}
	c.strs[s] = addr
	return addr, nil
}

func (c *Compiler) loadImm(r string, v int) error {
	if v < -(1<<31) || v >= 1<<31 {
		return fmt.Errorf("constant %d does not fit in 32 bits", v)
	}
	if v >= -32768 && v <= 32767 {
		c.emit(isa.Instruction{Opcode: "addi", Reg: r, Reg1: "zero", Value: v})
		return nil
	}
	lo := int(int16(v))
	hi := int(int16((v - lo) >> 16))
	c.emit(isa.Instruction{Opcode: "lui", Reg: r, Value: hi})
	if lo != 0 {
		c.emit(isa.Instruction{Opcode: "addi", Reg: r, Reg1: r, Value: lo})
	}
	return nil
}

func argc(v []any, min, max int) error {
	if len(v) < min || len(v) > max {
		return fmt.Errorf("Operation %q: invalid number of arguments (%d)", v[0], len(v)-1)
	}
	return nil
}

func isCmp(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func (c *Compiler) Compile(program []any) ([]isa.Instruction, []byte, error) {
	c.emitJump(isa.Instruction{Opcode: "j"}, "main")
	c.emitJump(isa.Instruction{Opcode: "j"}, "irq_handler")
	c.emitLabel("main")

	target, err := c.allocReg()
	if err != nil {
		return nil, nil, err
	}

	var handler []any
	for _, expr := range program {
		if l, ok := expr.([]any); ok && len(l) > 0 && l[0] == "interrupt" {
			if handler != nil {
				return nil, nil, fmt.Errorf("only one interrupt handler is allowed")
			}
			handler = l
			continue
		}
		if err := c.gen(expr, target); err != nil {
			return nil, nil, err
		}
	}

	c.freeReg(target)
	c.emit(isa.Instruction{Opcode: "halt"})

	c.emitLabel("irq_handler")
	if handler != nil {
		if err := c.genHandler(handler); err != nil {
			return nil, nil, err
		}
	}
	c.emit(isa.Instruction{Opcode: "iret"})
	code, err := c.resolveLabels()
	return code, c.data, err
}

func (c *Compiler) gen(expr any, target string) error {
	switch v := expr.(type) {
	case int:
		return c.loadImm(target, v)
	case parser.Str:
		addr, err := c.internString(string(v))
		if err != nil {
			return err
		}
		return c.loadImm(target, addr)
	case string:
		addr, ok := c.symbols[v]
		if !ok {
			return fmt.Errorf("Undefined variable %q", v)
		}
		c.emit(isa.Instruction{Opcode: "lw", Reg: target, Reg1: "zero", Value: addr})
		return nil
	case []any:
		return c.genList(v, target)
	default:
		return fmt.Errorf("Unsupported expression type: %T", expr)
	}
}

func (c *Compiler) genList(v []any, target string) error {
	if len(v) == 0 {
		return fmt.Errorf("Empty expression ()")
	}
	op, ok := v[0].(string)
	if !ok {
		return fmt.Errorf("The first element of the expression must be an operation, not %v", v[0])
	}

	if _, isBin := binOps[op]; isBin {
		return c.genBinary(op, v, target)
	}
	if isCmp(op) {
		return c.genCompare(op, v, target)
	}

	switch op {
	case "setq":
		return c.genSetq(v, target)
	case "print":
		return c.genPrint(v, target)
	case "load", "loadb":
		return c.genLoad(op, v, target)
	case "store", "storeb":
		return c.genStore(op, v, target)
	case "if":
		return c.genIf(v, target)
	case "while":
		return c.genWhile(v, target)
	case "progn":
		return c.genProgn(v, target)
	case "in":
		return c.genIn(v, target)
	case "out":
		return c.genOut(v, target)
	case "ei", "di":
		return c.genIntFlag(op, v, target)
	case "interrupt":
		return fmt.Errorf("interrupt must be a top-level form")
	default:
		return fmt.Errorf("неизвестная операция %q", op)
	}
}

func (c *Compiler) genOperands(v []any, target string) (string, error) {
	if err := argc(v, 3, 3); err != nil {
		return "", err
	}
	if err := c.gen(v[1], target); err != nil {
		return "", err
	}
	tmp, err := c.allocReg()
	if err != nil {
		return "", err
	}
	if err := c.gen(v[2], tmp); err != nil {
		return "", err
	}
	return tmp, nil
}

func (c *Compiler) genBinary(op string, v []any, target string) error {
	tmp, err := c.genOperands(v, target)
	if err != nil {
		return err
	}
	c.emit(isa.Instruction{Opcode: binOps[op], Reg: target, Reg1: target, Reg2: tmp})
	c.freeReg(tmp)
	return nil
}

func (c *Compiler) genCompare(op string, v []any, target string) error {
	tmp, err := c.genOperands(v, target)
	if err != nil {
		return err
	}
	defer c.freeReg(tmp)

	br, a, b := "", target, tmp
	switch op {
	case "<":
		br = "blt"
	case ">":
		br, a, b = "blt", tmp, target
	case "<=":
		br, a, b = "bge", tmp, target
	case ">=":
		br = "bge"
	case "==":
		br = "beq"
	case "!=":
		br = "bne"
	}

	trueL, endL := c.newLabel("cmp_true"), c.newLabel("cmp_end")
	c.emitJump(isa.Instruction{Opcode: br, Reg1: a, Reg2: b}, trueL)
	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: "zero", Value: 0})
	c.emitJump(isa.Instruction{Opcode: "j"}, endL)
	c.emitLabel(trueL)
	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: "zero", Value: 1})
	c.emitLabel(endL)
	return nil
}

func (c *Compiler) genJumpIfFalse(expr any, label string) error {
	if l, ok := expr.([]any); ok && len(l) == 3 {
		if op, _ := l[0].(string); isCmp(op) {

			r1, err := c.allocReg()
			if err != nil {
				return err
			}

			defer c.freeReg(r1)
			if err := c.gen(l[1], r1); err != nil {
				return err
			}

			r2, err := c.allocReg()
			if err != nil {
				return err
			}

			defer c.freeReg(r2)
			if err := c.gen(l[2], r2); err != nil {
				return err
			}

			var br, a, b string
			switch op {
			case "<":
				br, a, b = "bge", r1, r2
			case ">":
				br, a, b = "bge", r2, r1
			case "<=":
				br, a, b = "blt", r2, r1
			case ">=":
				br, a, b = "blt", r1, r2
			case "==":
				br, a, b = "bne", r1, r2
			case "!=":
				br, a, b = "beq", r1, r2
			}
			c.emitJump(isa.Instruction{Opcode: br, Reg1: a, Reg2: b}, label)
			return nil
		}
	}

	r, err := c.allocReg()

	if err != nil {
		return err
	}

	defer c.freeReg(r)

	if err := c.gen(expr, r); err != nil {
		return err
	}

	c.emitJump(isa.Instruction{Opcode: "beqz", Reg1: r}, label)
	return nil
}

func (c *Compiler) genSetq(v []any, target string) error {
	if err := argc(v, 3, 3); err != nil {
		return err
	}

	name, ok := v[1].(string)

	if !ok {
		return fmt.Errorf("setq: variable name must be an identifier")
	}
	if err := c.gen(v[2], target); err != nil {
		return err
	}

	addr, err := c.declare(name)

	if err != nil {
		return err
	}

	c.emit(isa.Instruction{Opcode: "sw", Reg: target, Reg1: "zero", Value: addr})
	return nil
}

func (c *Compiler) genRead(v []any, target string) error {
	if err := argc(v, 2, 2); err != nil {
		return err
	}

	name, ok := v[1].(string)

	if !ok {
		return fmt.Errorf("read: argument must be an identifier")
	}
	addr, err := c.declare(name)

	if err != nil {
		return err
	}

	c.emit(isa.Instruction{Opcode: "IN", Reg: target, Value: 0})
	c.emit(isa.Instruction{Opcode: "sw", Reg: target, Reg1: "zero", Addr: addr})
	return nil
}

func (c *Compiler) genPrint(v []any, target string) error {
	if err := argc(v, 2, 2); err != nil {
		return err
	}
	if err := c.gen(v[1], target); err != nil {
		return err
	}

	c.emit(isa.Instruction{Opcode: "out", Reg: target, Value: isa.PortOutData})
	return nil
}

func (c *Compiler) genLoad(op string, v []any, target string) error {
	if err := argc(v, 2, 2); err != nil {
		return err
	}
	if err := c.gen(v[1], target); err != nil {
		return err
	}
	opcode := "lw"
	if op == "loadb" {
		opcode = "lb"
	}
	c.emit(isa.Instruction{Opcode: opcode, Reg: target, Reg1: target})
	return nil
}

func (c *Compiler) genStore(op string, v []any, target string) error {
	tmp, err := c.genOperands(v, target)
	if err != nil {
		return err
	}
	opcode := "sw"
	if op == "storeb" {
		opcode = "sb"
	}
	c.emit(isa.Instruction{Opcode: opcode, Reg: tmp, Reg1: target})
	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: tmp})
	c.freeReg(tmp)
	return nil
}

func (c *Compiler) genIn(v []any, target string) error {
	if err := argc(v, 2, 2); err != nil {
		return err
	}
	port, ok := v[1].(int)
	if !ok {
		return fmt.Errorf("in: port must be an integer literal")
	}
	c.emit(isa.Instruction{Opcode: "in", Reg: target, Value: port})
	return nil
}

func (c *Compiler) genOut(v []any, target string) error {
	if err := argc(v, 3, 3); err != nil {
		return err
	}
	port, ok := v[1].(int)
	if !ok {
		return fmt.Errorf("out: port must be an integer literal")
	}
	if err := c.gen(v[2], target); err != nil {
		return err
	}
	c.emit(isa.Instruction{Opcode: "out", Reg: target, Value: port})
	return nil
}

func (c *Compiler) genIntFlag(op string, v []any, target string) error {
	if err := argc(v, 1, 1); err != nil {
		return err
	}
	c.emit(isa.Instruction{Opcode: op})
	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: "zero", Value: 0})
	return nil
}

func (c *Compiler) genHandler(v []any) error {
	saved := c.freeRegs
	c.freeRegs = []string{"s1", "s2", "s3", "s4", "s5", "s6"}
	defer func() { c.freeRegs = saved }()

	target, err := c.allocReg()
	if err != nil {
		return err
	}
	for _, e := range v[1:] {
		if err := c.gen(e, target); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) genProgn(v []any, target string) error {
	for _, e := range v[1:] {
		if err := c.gen(e, target); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) genIf(v []any, target string) error {
	if err := argc(v, 3, 4); err != nil {
		return err
	}

	elseL, endL := c.newLabel("else"), c.newLabel("endif")

	if err := c.genJumpIfFalse(v[1], elseL); err != nil {
		return err
	}
	if err := c.gen(v[2], target); err != nil {
		return err
	}

	c.emitJump(isa.Instruction{Opcode: "j"}, endL)

	c.emitLabel(elseL)
	if len(v) == 4 {
		if err := c.gen(v[3], target); err != nil {
			return err
		}
	}
	c.emitLabel(endL)
	return nil
}

func (c *Compiler) genWhile(v []any, target string) error {
	if err := argc(v, 2, 1<<30); err != nil {
		return err
	}

	startL, endL := c.newLabel("while_start"), c.newLabel("while_end")

	c.emitLabel(startL)
	if err := c.genJumpIfFalse(v[1], endL); err != nil {
		return err
	}

	for _, stmt := range v[2:] {
		if err := c.gen(stmt, target); err != nil {
			return err
		}
	}

	c.emitJump(isa.Instruction{Opcode: "j"}, startL)
	c.emitLabel(endL)
	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: "zero", Value: 0}) // while даёт 0
	return nil
}

func (c *Compiler) resolveLabels() ([]isa.Instruction, error) {
	positions := make(map[string]int)
	var out []isa.Instruction
	var targets []string

	for _, it := range c.code {
		if it.label != "" {
			positions[it.label] = len(out)
			continue
		}
		out = append(out, it.ins)
		targets = append(targets, it.target)
	}
	for i, t := range targets {
		if t == "" {
			continue
		}
		addr, ok := positions[t]

		if !ok {
			return nil, fmt.Errorf("неизвестная метка %q", t)
		}

		out[i].Addr = addr * 4
	}
	return out, nil
}
