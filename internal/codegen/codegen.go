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
	code      []item
	symbols   map[string]int
	data      []byte
	strs      map[string]int
	labelCnt  int
	freeRegs  []string
	funcs     map[string]*funcInfo
	funcOrder []*funcInfo
	fn        *frame
	spillAddr int
	inHandler bool
}

func New() *Compiler {
	return &Compiler{
		symbols:  make(map[string]int),
		strs:     make(map[string]int),
		funcs:    make(map[string]*funcInfo),
		freeRegs: append([]string(nil), tempRegs...),
	}
}

var tempRegs = []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6"}

const frameHeader = 4 + 4*7

type funcInfo struct {
	name   string
	params []string
	body   []any
	label  string
}

type frame struct {
	locals  map[string]int
	nextOff int
}

var builtins = map[string]bool{
	"setq": true, "print": true, "if": true, "while": true, "progn": true,
	"in": true, "out": true, "ei": true, "di": true, "interrupt": true,
	"load": true, "loadb": true, "store": true, "storeb": true, "defun": true,
}

func isBuiltin(op string) bool {
	_, bin := binOps[op]
	return bin || isCmp(op) || builtins[op]
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
	if addr+len(b) > isa.DataMemSize-isa.StackSize {
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
	var body []any
	var handler []any
	for _, expr := range program {
		if l, ok := expr.([]any); ok && len(l) > 0 {
			switch l[0] {
			case "interrupt":
				if handler != nil {
					return nil, nil, fmt.Errorf("only one interrupt handler is allowed")
				}
				handler = l
				continue
			case "defun":
				if err := c.registerFunc(l); err != nil {
					return nil, nil, err
				}
				continue
			}
		}
		body = append(body, expr)
	}

	var err error
	c.spillAddr, err = c.alloc(make([]byte, 4*len(tempRegs)))
	if err != nil {
		return nil, nil, err
	}

	c.emitJump(isa.Instruction{Opcode: "j"}, "main")
	c.emitJump(isa.Instruction{Opcode: "j"}, "irq_handler")
	c.emitLabel("main")
	if err := c.loadImm("sp", isa.DataMemSize); err != nil {
		return nil, nil, err
	}

	target, err := c.allocReg()
	if err != nil {
		return nil, nil, err
	}
	for _, expr := range body {
		if err := c.gen(expr, target); err != nil {
			return nil, nil, err
		}
	}
	c.freeReg(target)
	c.emit(isa.Instruction{Opcode: "halt"})

	for _, f := range c.funcOrder {
		if err := c.genFunc(f); err != nil {
			return nil, nil, err
		}
	}

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
		base, off, ok := c.resolveVar(v)
		if !ok {
			return fmt.Errorf("Undefined variable %q", v)
		}
		c.emit(isa.Instruction{Opcode: "lw", Reg: target, Reg1: base, Value: off})
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
	case "defun":
		return fmt.Errorf("defun must be a top-level form")
	default:
		if f, ok := c.funcs[op]; ok {
			return c.genCall(f, v, target)
		}
		return fmt.Errorf("Unknown operation %q", op)
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

	base, off, err := c.declareVar(name)
	if err != nil {
		return err
	}
	c.emit(isa.Instruction{Opcode: "sw", Reg: target, Reg1: base, Value: off})
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
	c.inHandler = true
	defer func() { c.inHandler = false }()
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

func (c *Compiler) resolveVar(name string) (base string, off int, ok bool) {
	if c.fn != nil {
		if o, found := c.fn.locals[name]; found {
			return "sp", o, true
		}
	}
	if a, found := c.symbols[name]; found {
		return "zero", a, true
	}
	return "", 0, false
}

func (c *Compiler) declareVar(name string) (string, int, error) {
	if base, off, ok := c.resolveVar(name); ok {
		return base, off, nil
	}
	if c.fn != nil {
		off := c.fn.nextOff
		c.fn.nextOff += 4
		c.fn.locals[name] = off
		return "sp", off, nil
	}
	addr, err := c.declare(name)
	return "zero", addr, err
}

func (c *Compiler) registerFunc(v []any) error {
	if len(v) < 4 {
		return fmt.Errorf("defun: expected (defun name (params) body...)")
	}
	name, ok := v[1].(string)
	if !ok {
		return fmt.Errorf("defun: function name must be an identifier")
	}
	if isBuiltin(name) {
		return fmt.Errorf("defun: %q is a reserved name", name)
	}
	if _, dup := c.funcs[name]; dup {
		return fmt.Errorf("defun: function %q is already defined", name)
	}
	plist, ok := v[2].([]any)
	if !ok {
		return fmt.Errorf("defun %s: parameter list expected", name)
	}
	if len(plist) > 8 {
		return fmt.Errorf("defun %s: at most 8 parameters are supported", name)
	}
	seen := map[string]bool{}
	params := make([]string, 0, len(plist))
	for _, p := range plist {
		s, ok := p.(string)
		if !ok || seen[s] {
			return fmt.Errorf("defun %s: bad or duplicate parameter %v", name, p)
		}
		seen[s] = true
		params = append(params, s)
	}
	f := &funcInfo{name: name, params: params, body: v[3:], label: "fn_" + name}
	c.funcs[name] = f
	c.funcOrder = append(c.funcOrder, f)
	return nil
}

func (c *Compiler) genFunc(f *funcInfo) error {
	c.fn = &frame{locals: map[string]int{}, nextOff: frameHeader}
	defer func() { c.fn = nil }()

	for _, p := range f.params {
		c.fn.locals[p] = c.fn.nextOff
		c.fn.nextOff += 4
	}

	c.emitLabel(f.label)
	c.emit(isa.Instruction{Opcode: "addi", Reg: "sp", Reg1: "sp"}) // размер допишем в конце
	prologue := len(c.code) - 1
	c.emit(isa.Instruction{Opcode: "sw", Reg: "ra", Reg1: "sp", Value: 0})
	for i, p := range f.params {
		c.emit(isa.Instruction{Opcode: "sw", Reg: fmt.Sprintf("a%d", i), Reg1: "sp", Value: c.fn.locals[p]})
	}

	target, err := c.allocReg()
	if err != nil {
		return err
	}
	for _, e := range f.body {
		if err := c.gen(e, target); err != nil {
			return fmt.Errorf("in function %s: %w", f.name, err)
		}
	}
	c.emit(isa.Instruction{Opcode: "addi", Reg: "a0", Reg1: target}) // результат = значение последнего выражения
	c.freeReg(target)

	size := c.fn.nextOff
	c.code[prologue].ins.Value = -size
	c.emit(isa.Instruction{Opcode: "lw", Reg: "ra", Reg1: "sp", Value: 0})
	c.emit(isa.Instruction{Opcode: "addi", Reg: "sp", Reg1: "sp", Value: size})
	c.emit(isa.Instruction{Opcode: "jalr", Reg: "zero", Reg1: "ra", Value: 0})
	return nil
}

func (c *Compiler) liveTemps(except string) []string {
	free := map[string]bool{}
	for _, r := range c.freeRegs {
		free[r] = true
	}
	var live []string
	for _, r := range tempRegs {
		if !free[r] && r != except {
			live = append(live, r)
		}
	}
	return live
}

func (c *Compiler) spillOp(op string, regs []string) {
	for _, r := range regs {
		i := 0
		for j, t := range tempRegs {
			if t == r {
				i = j
			}
		}
		base, off := "zero", c.spillAddr+4*i
		if c.fn != nil {
			base, off = "sp", 4+4*i
		}
		c.emit(isa.Instruction{Opcode: op, Reg: r, Reg1: base, Value: off})
	}
}

func (c *Compiler) genCall(f *funcInfo, v []any, target string) error {
	if c.inHandler {
		return fmt.Errorf("function %q cannot be called from an interrupt handler", f.name)
	}
	if len(v)-1 != len(f.params) {
		return fmt.Errorf("function %q expects %d argument(s), got %d", f.name, len(f.params), len(v)-1)
	}

	var regs []string
	for _, a := range v[1:] {
		r, err := c.allocReg()
		if err != nil {
			return err
		}
		regs = append(regs, r)
		if err := c.gen(a, r); err != nil {
			return err
		}
	}
	for i, r := range regs {
		c.emit(isa.Instruction{Opcode: "addi", Reg: fmt.Sprintf("a%d", i), Reg1: r})
	}
	for i := len(regs) - 1; i >= 0; i-- {
		c.freeReg(regs[i])
	}

	live := c.liveTemps(target)
	c.spillOp("sw", live)
	c.emitJump(isa.Instruction{Opcode: "jal", Reg: "ra"}, f.label)
	c.spillOp("lw", live)

	c.emit(isa.Instruction{Opcode: "addi", Reg: target, Reg1: "a0"})
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
