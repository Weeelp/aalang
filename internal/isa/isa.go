package isa

type Instruction struct {
	Opcode string `json:"opcode"`
	Reg    string `json:"reg,omitempty"`
	Reg1   string `json:"reg1,omitempty"`
	Reg2   string `json:"reg2,omitempty"`
	Addr   int    `json:"addr,omitempty"`
	Value  int    `json:"value,omitempty"`
}
