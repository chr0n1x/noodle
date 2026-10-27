package dispatcher

import "testing"

func TestCloseStdinAfterTurn(t *testing.T) {
	cases := []struct {
		name          string
		exitAfterTurn bool
		keepAlive     bool
		want          bool
	}{
		{name: "default keeps stdin open", want: false},
		{name: "opt-in closes it for work agents", exitAfterTurn: true, want: true},
		{name: "keep-alive sessions stay open", exitAfterTurn: true, keepAlive: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewProcessDispatcher(ProcessDispatcherConfig{
				ProviderConfigs: ProviderConfigs{Claude: ProviderConfig{ExitAfterTurn: tc.exitAfterTurn}},
			})
			got := d.closeStdinAfterTurn(DispatchRequest{KeepAlive: tc.keepAlive})
			if got != tc.want {
				t.Fatalf("closeStdinAfterTurn = %v, want %v", got, tc.want)
			}
		})
	}
}
