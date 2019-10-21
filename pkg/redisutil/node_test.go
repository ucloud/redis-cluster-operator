package redisutil

import (
	"reflect"
	"testing"
)

func TestNodes_SortByFunc(t *testing.T) {
	n1 := Node{
		ID:      "n1",
		IP:      "10.1.1.1",
		Port:    "",
		Role:    "master",
		balance: 1365,
	}
	n2 := Node{
		ID:      "n2",
		IP:      "10.1.1.2",
		Port:    "",
		Role:    "master",
		balance: 1366,
	}
	n3 := Node{
		ID:      "n3",
		IP:      "10.1.1.3",
		Port:    "",
		Role:    "master",
		balance: 1365,
	}
	n4 := Node{
		ID:      "n4",
		IP:      "10.1.1.4",
		Port:    "",
		Role:    "master",
		balance: -4096,
	}
	type args struct {
		less func(*Node, *Node) bool
	}
	tests := []struct {
		name string
		n    Nodes
		args args
		want Nodes
	}{
		{
			name: "asc by balance",
			n:    Nodes{&n1, &n2, &n3, &n4},
			args: args{less: func(a, b *Node) bool { return a.Balance() < b.Balance() }},
			want: Nodes{&n4, &n1, &n3, &n2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.n.SortByFunc(tt.args.less); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SortByFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}
