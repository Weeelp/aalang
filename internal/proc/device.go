package proc

type InputEvent struct {
	Tick int
	Char byte
}

type Device struct {
	Schedule  []InputEvent
	next      int
	DataReg   byte
	DataValid bool
	Output    []byte
	Overruns  int
}

func (d *Device) poll(tick int) (raised bool, overrun int) {
	for d.next < len(d.Schedule) && d.Schedule[d.next].Tick <= tick {
		ev := d.Schedule[d.next]
		d.next++
		if d.DataValid {
			d.Overruns++
			overrun++
		}
		d.DataReg = ev.Char
		d.DataValid = true
		raised = true
	}
	return raised, overrun
}

func (d *Device) read() (val int, wasValid bool) {
	wasValid = d.DataValid
	d.DataValid = false
	return int(d.DataReg), wasValid
}

func BuildSchedule(input []byte, first, period int) []InputEvent {
	ev := make([]InputEvent, 0, len(input)+1)
	for i, b := range input {
		ev = append(ev, InputEvent{Tick: first + i*period, Char: b})
	}
	ev = append(ev, InputEvent{Tick: first + len(input)*period, Char: 0})
	return ev
}
