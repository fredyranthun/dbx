package bubbletea

type Msg interface{}

type Cmd func() Msg

type Model interface {
	Init() Cmd
	Update(Msg) (Model, Cmd)
	View() string
}

type ProgramOption interface {
	apply(*Program)
}

type Program struct {
	model Model
}

func NewProgram(model Model, _ ...ProgramOption) *Program {
	return &Program{model: model}
}

func (p *Program) Run() (Model, error) {
	if p.model == nil {
		return nil, nil
	}
	if initCmd := p.model.Init(); initCmd != nil {
		_ = initCmd()
	}
	return p.model, nil
}

type WindowSizeMsg struct {
	Width  int
	Height int
}

type KeyMsg struct {
	Value string
}

func (k KeyMsg) String() string {
	return k.Value
}

var Quit Cmd = func() Msg { return nil }
