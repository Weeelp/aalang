package codegen

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/parser"
)

type compileError string

type branch struct {
	op   string
	swap bool
}

var (
	binOps = map[string]string{"+": "add", "-": "sub", "*": "mul", "/": "div", "%": "rem"}
	memOps = map[string]string{"load": "lw", "loadb": "lb", "store": "sw", "storeb": "sb"}

	falseBranch = map[string]branch{
		"<": {"bge", false}, ">": {"bge", true}, "<=": {"blt", true},
		">=": {"blt", false}, "==": {"bne", false}, "!=": {"beq", false},
	}

	tempRegs    = []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6"}
	handlerRegs = []string{"s1", "s2", "s3", "s4", "s5", "s6"}

	forms map[string]func(*Compiler, []any, string)
)

func init() {
	topLevel := func(c *Compiler, v []any, _ string) { c.fail("%v must be a top-level form", v[0]) }
	forms = map[string]func(*Compiler, []any, string){
		"setq": (*Compiler).genSetq,
		"if":   (*Compiler).genIf, "while": (*Compiler).genWhile,
		"in": (*Compiler).genIn, "out": (*Compiler).genOut,
		"ei": (*Compiler).genFlag, "di": (*Compiler).genFlag,
		"interrupt": topLevel, "defun": topLevel,
		"progn": func(c *Compiler, v []any, t string) { c.genBody(v[1:], t) },
		"print": func(c *Compiler, v []any, t string) {
			c.need(v, 1, 1<<30)
			for _, a := range v[1:] {
				if s, ok := a.(parser.Str); ok {
					c.genPuts(s, t)
				} else {
					c.genOut([]any{"out", isa.PortOutData, a}, t)
				}
			}
		},
		"puts": func(c *Compiler, v []any, t string) {
			c.need(v, 1, 1)
			c.genPuts(v[1], t)
		},
	}
	for op := range binOps {
		forms[op] = (*Compiler).genBinary
	}
	for op := range memOps {
		forms[op] = (*Compiler).genMem
	}
	for op := range falseBranch {
		forms[op] = func(c *Compiler, v []any, t string) { c.genIf([]any{"if", v, 1, 0}, t) }
	}
}

type function struct {
	params []string
	body   []any
	addr   int
}

type fixup struct {
	idx  int
	name string
}

type Compiler struct {
	code      []isa.Instruction
	data      []byte
	symbols   map[string]int
	strs      map[string]int
	funcs     map[string]*function
	order     []string
	fixups    []fixup
	pool      []string
	busy      map[string]bool
	locals    map[string]int
	frame     int
	inHandler bool
}

func New() *Compiler {
	return &Compiler{
		symbols: map[string]int{},
		strs:    map[string]int{},
		funcs:   map[string]*function{},
		busy:    map[string]bool{},
		pool:    slices.Clone(tempRegs),
	}
}

func (c *Compiler) fail(format string, a ...any) {
	panic(compileError(fmt.Sprintf(format, a...)))
}

func (c *Compiler) need(v []any, min, max int) {
	if n := len(v) - 1; n < min || n > max {
		c.fail("%v: wrong number of arguments (%d)", v[0], n)
	}
}

func (c *Compiler) emit(in isa.Instruction) int {
	c.code = append(c.code, in)
	return len(c.code) - 1
}

func (c *Compiler) i(op, rd, rs1 string, imm int) int {
	return c.emit(isa.Instruction{Opcode: op, Reg: rd, Reg1: rs1, Value: imm})
}

func (c *Compiler) jump(op, r1, r2 string) int {
	return c.emit(isa.Instruction{Opcode: op, Reg1: r1, Reg2: r2})
}

func (c *Compiler) here() int { return len(c.code) * 4 }

func (c *Compiler) patch(idx int) { c.code[idx].Addr = c.here() }

func (c *Compiler) reg() string {
	if len(c.pool) == 0 {
		c.fail("expression too nested: out of temporary registers")
	}
	r := c.pool[0]
	c.pool = c.pool[1:]
	c.busy[r] = true
	return r
}

func (c *Compiler) free(r string) {
	delete(c.busy, r)
	c.pool = append([]string{r}, c.pool...)
}

func (c *Compiler) alloc(b []byte) int {
	for len(c.data)%4 != 0 {
		c.data = append(c.data, 0)
	}
	addr := len(c.data)
	if addr+len(b) > isa.DataMemSize-isa.StackSize {
		c.fail("data memory has run out")
	}
	c.data = append(c.data, b...)
	return addr
}

func (c *Compiler) str(s string) int {
	if addr, ok := c.strs[s]; ok {
		return addr
	}
	c.strs[s] = c.alloc(append([]byte(s), 0))
	return c.strs[s]
}

func (c *Compiler) loadImm(r string, v int) {
	if v != int(int32(v)) {
		c.fail("constant %d does not fit in 32 bits", v)
	}
	if v == int(int16(v)) {
		c.i("addi", r, "zero", v)
		return
	}
	lo := int(int16(v))
	c.i("lui", r, "", int(int16((v-lo)>>16)))
	c.i("addi", r, r, lo)
}

func (c *Compiler) lookup(name string) (base string, off int, ok bool) {
	if off, ok := c.locals[name]; ok {
		return "sp", off, true
	}
	if addr, ok := c.symbols[name]; ok {
		return "zero", addr, true
	}
	return "", 0, false
}

func (c *Compiler) declare(name string) (string, int) {
	if base, off, ok := c.lookup(name); ok {
		return base, off
	}
	if c.locals != nil {
		c.locals[name] = c.frame
		c.frame += 4
		return "sp", c.locals[name]
	}
	c.symbols[name] = c.alloc(make([]byte, 4))
	return "zero", c.symbols[name]
}

func (c *Compiler) gen(e any, t string) {
	switch v := e.(type) {
	case int:
		c.loadImm(t, v)
	case parser.Str:
		c.loadImm(t, c.str(string(v)))
	case string:
		base, off, ok := c.lookup(v)
		if !ok {
			c.fail("undefined variable %q", v)
		}
		c.i("lw", t, base, off)
	case []any:
		c.genList(v, t)
	default:
		c.fail("unsupported expression %T", e)
	}
}

func (c *Compiler) genList(v []any, t string) {
	if len(v) == 0 {
		c.fail("empty expression ()")
	}
	op, ok := v[0].(string)
	if !ok {
		c.fail("operation expected, got %v", v[0])
	}
	if f, ok := forms[op]; ok {
		f(c, v, t)
	} else if fn, ok := c.funcs[op]; ok {
		c.call(op, fn, v, t)
	} else {
		c.fail("unknown operation %q", op)
	}
}

func (c *Compiler) genBody(es []any, t string) {
	for _, e := range es {
		c.gen(e, t)
	}
}

func (c *Compiler) operands(v []any, t string) string {
	c.need(v, 2, 2)
	c.gen(v[1], t)
	tmp := c.reg()
	c.gen(v[2], tmp)
	return tmp
}

func (c *Compiler) genBinary(v []any, t string) {
	tmp := c.operands(v, t)
	c.emit(isa.Instruction{Opcode: binOps[v[0].(string)], Reg: t, Reg1: t, Reg2: tmp})
	c.free(tmp)
}

func (c *Compiler) jumpIfFalse(e any) int {
	r := c.reg()
	defer c.free(r)
	if l, ok := e.([]any); ok && len(l) == 3 {
		op, _ := l[0].(string)
		if br := falseBranch[op]; br.op != "" {
			tmp := c.operands(l, r)
			defer c.free(tmp)
			x, y := r, tmp
			if br.swap {
				x, y = y, x
			}
			return c.jump(br.op, x, y)
		}
	}
	c.gen(e, r)
	return c.jump("beqz", r, "")
}

func (c *Compiler) genIf(v []any, t string) {
	c.need(v, 2, 3)
	jf := c.jumpIfFalse(v[1])
	c.gen(v[2], t)
	je := c.jump("j", "", "")
	c.patch(jf)
	if len(v) == 4 {
		c.gen(v[3], t)
	}
	c.patch(je)
}

func (c *Compiler) genWhile(v []any, t string) {
	c.need(v, 1, 1<<30)
	start := c.here()
	jf := c.jumpIfFalse(v[1])
	c.genBody(v[2:], t)
	c.emit(isa.Instruction{Opcode: "j", Addr: start})
	c.patch(jf)
	c.i("addi", t, "zero", 0)
}

func (c *Compiler) genSetq(v []any, t string) {
	c.need(v, 2, 2)
	name, ok := v[1].(string)
	if !ok {
		c.fail("setq: variable name must be an identifier")
	}
	c.gen(v[2], t)
	base, off := c.declare(name)
	c.i("sw", t, base, off)
}

func (c *Compiler) genMem(v []any, t string) {
	op := memOps[v[0].(string)]
	if op[0] == 'l' {
		c.need(v, 1, 1)
		c.gen(v[1], t)
		c.i(op, t, t, 0)
		return
	}
	tmp := c.operands(v, t)
	c.i(op, tmp, t, 0)
	c.i("addi", t, tmp, 0)
	c.free(tmp)
}

func (c *Compiler) port(x any) int {
	p, ok := x.(int)
	if !ok {
		c.fail("port must be an integer literal")
	}
	return p
}

func (c *Compiler) genIn(v []any, t string) {
	c.need(v, 1, 1)
	c.emit(isa.Instruction{Opcode: "in", Reg: t, Value: c.port(v[1])})
}

func (c *Compiler) genOut(v []any, t string) {
	c.need(v, 2, 2)
	c.gen(v[2], t)
	c.emit(isa.Instruction{Opcode: "out", Reg: t, Value: c.port(v[1])})
}

func (c *Compiler) genPuts(e any, t string) {
	p, ch := c.reg(), c.reg()
	c.gen(e, p)
	start := c.here()
	c.i("lb", ch, p, 0)
	end := c.jump("beqz", ch, "")
	c.emit(isa.Instruction{Opcode: "out", Reg: ch, Value: isa.PortOutData})
	c.i("addi", p, p, 1)
	c.emit(isa.Instruction{Opcode: "j", Addr: start})
	c.patch(end)
	c.i("addi", t, "zero", 0)
	c.free(ch)
	c.free(p)
}

func (c *Compiler) genFlag(v []any, t string) {
	c.need(v, 0, 0)
	c.emit(isa.Instruction{Opcode: v[0].(string)})
	c.i("addi", t, "zero", 0)
}

func (c *Compiler) genHandler(v []any) {
	saved := c.pool
	c.pool, c.inHandler = slices.Clone(handlerRegs), true
	c.genBody(v[1:], c.reg())
	c.pool, c.inHandler = saved, false
}

func (c *Compiler) defun(v []any) {
	if len(v) < 4 {
		c.fail("defun: expected (defun name (params) body...)")
	}
	name, _ := v[1].(string)
	plist, ok := v[2].([]any)
	if name == "" || !ok || len(plist) > 8 {
		c.fail("defun: bad name or parameter list (at most 8 parameters)")
	}
	if _, reserved := forms[name]; reserved || c.funcs[name] != nil {
		c.fail("defun: %q is reserved or already defined", name)
	}
	f := &function{body: v[3:]}
	for _, p := range plist {
		s, ok := p.(string)
		if !ok || slices.Contains(f.params, s) {
			c.fail("defun %s: bad or duplicate parameter %v", name, p)
		}
		f.params = append(f.params, s)
	}
	c.funcs[name] = f
	c.order = append(c.order, name)
}

func (c *Compiler) genFunc(f *function) {
	f.addr = c.here()
	c.locals, c.frame = map[string]int{}, 4
	defer func() { c.locals = nil }()

	prologue := c.i("addi", "sp", "sp", 0)
	c.i("sw", "ra", "sp", 0)
	for i, p := range f.params {
		c.locals[p] = c.frame
		c.i("sw", fmt.Sprintf("a%d", i), "sp", c.frame)
		c.frame += 4
	}
	t := c.reg()
	c.genBody(f.body, t)
	c.i("addi", "a0", t, 0)
	c.free(t)

	c.code[prologue].Value = -c.frame
	c.i("lw", "ra", "sp", 0)
	c.i("addi", "sp", "sp", c.frame)
	c.emit(isa.Instruction{Opcode: "jalr", Reg: "zero", Reg1: "ra"})
}

func (c *Compiler) call(name string, f *function, v []any, t string) {
	if c.inHandler {
		c.fail("function %q cannot be called from an interrupt handler", name)
	}
	if len(v)-1 != len(f.params) {
		c.fail("function %q expects %d argument(s), got %d", name, len(f.params), len(v)-1)
	}
	args := make([]string, len(f.params))
	for i, a := range v[1:] {
		args[i] = c.reg()
		c.gen(a, args[i])
	}
	for i, r := range args {
		c.i("addi", fmt.Sprintf("a%d", i), r, 0)
	}
	for _, arg := range slices.Backward(args) {
		c.free(arg)
	}

	var live []string
	for _, r := range tempRegs {
		if c.busy[r] && r != t {
			live = append(live, r)
		}
	}
	n := 4 * len(live)
	if n > 0 {
		c.i("addi", "sp", "sp", -n)
	}
	for i, r := range live {
		c.i("sw", r, "sp", 4*i)
	}
	c.fixups = append(c.fixups, fixup{c.emit(isa.Instruction{Opcode: "jal", Reg: "ra"}), name})
	for i, r := range live {
		c.i("lw", r, "sp", 4*i)
	}
	if n > 0 {
		c.i("addi", "sp", "sp", n)
	}
	c.i("addi", t, "a0", 0)
}

func (c *Compiler) Compile(prog []any) (code []isa.Instruction, data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(compileError)
			if !ok {
				panic(r)
			}
			err = errors.New(string(msg))
		}
	}()

	var body, handler []any
	for _, e := range prog {
		l, _ := e.([]any)
		switch {
		case len(l) > 0 && l[0] == "interrupt":
			if handler != nil {
				c.fail("only one interrupt handler is allowed")
			}
			handler = l
		case len(l) > 0 && l[0] == "defun":
			c.defun(l)
		default:
			body = append(body, e)
		}
	}

	jmain := c.jump("j", "", "")
	jirq := c.jump("j", "", "")
	c.patch(jmain)
	c.loadImm("sp", isa.DataMemSize)
	t := c.reg()
	c.genBody(body, t)
	c.free(t)
	c.emit(isa.Instruction{Opcode: "halt"})

	for _, name := range c.order {
		c.genFunc(c.funcs[name])
	}

	c.patch(jirq)
	if handler != nil {
		c.genHandler(handler)
	}
	c.emit(isa.Instruction{Opcode: "iret"})

	for _, f := range c.fixups {
		c.code[f.idx].Addr = c.funcs[f.name].addr
	}
	return c.code, c.data, nil
}
