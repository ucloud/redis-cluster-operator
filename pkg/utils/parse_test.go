package utils

import "testing"

func TestParseRedisMemConf(t *testing.T) {
	type args struct {
		p string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "b",
			args: args{
				p: "12b",
			},
			want:    "12",
			wantErr: false,
		},
		{
			name: "digit",
			args: args{
				p: "1202",
			},
			want:    "1202",
			wantErr: false,
		},
		{
			name: "B",
			args: args{
				p: "12B",
			},
			want:    "12",
			wantErr: false,
		},
		{
			name: "k",
			args: args{
				p: "12k",
			},
			want:    "12000",
			wantErr: false,
		},
		{
			name: "kk",
			args: args{
				p: "12kk",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "kb",
			args: args{
				p: "12kb",
			},
			want:    "12288",
			wantErr: false,
		},
		{
			name: "Kb",
			args: args{
				p: "12Kb",
			},
			want:    "12288",
			wantErr: false,
		},
		{
			name: "m",
			args: args{
				p: "12m",
			},
			want:    "12000000",
			wantErr: false,
		},
		{
			name: "mB",
			args: args{
				p: "12mb",
			},
			want:    "12582912",
			wantErr: false,
		},
		{
			name: "g",
			args: args{
				p: "12g",
			},
			want:    "12000000000",
			wantErr: false,
		},
		{
			name: "gb",
			args: args{
				p: "12gb",
			},
			want:    "12884901888",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRedisMemConf(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRedisMemConf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRedisMemConf() got = %v, want %v", got, tt.want)
			}
		})
	}
}
