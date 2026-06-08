package cli

import "testing"

func TestMenuKey(t *testing.T) {
	const count = 3
	cases := []struct {
		name    string
		b       []byte
		cur     int
		wantCur int
		wantAct menuAction
	}{
		{"down arrow", []byte{0x1b, '[', 'B'}, 0, 1, menuNone},
		{"down arrow at end clamps", []byte{0x1b, '[', 'B'}, 2, 2, menuNone},
		{"up arrow", []byte{0x1b, '[', 'A'}, 2, 1, menuNone},
		{"up arrow at top clamps", []byte{0x1b, '[', 'A'}, 0, 0, menuNone},
		{"vim j down", []byte{'j'}, 0, 1, menuNone},
		{"vim k up", []byte{'k'}, 1, 0, menuNone},
		{"enter confirms", []byte{'\r'}, 1, 1, menuConfirm},
		{"newline confirms", []byte{'\n'}, 1, 1, menuConfirm},
		{"q cancels", []byte{'q'}, 1, 1, menuCancel},
		{"ctrl-c cancels", []byte{3}, 1, 1, menuCancel},
		{"bare esc cancels", []byte{0x1b}, 1, 1, menuCancel},
		{"empty cancels", []byte{}, 1, 1, menuCancel},
		{"other key no-op", []byte{'x'}, 1, 1, menuNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCur, gotAct := menuKey(c.b, c.cur, count)
			if gotCur != c.wantCur || gotAct != c.wantAct {
				t.Errorf("menuKey(%v, cur=%d) = (%d, %v), want (%d, %v)",
					c.b, c.cur, gotCur, gotAct, c.wantCur, c.wantAct)
			}
		})
	}
}
