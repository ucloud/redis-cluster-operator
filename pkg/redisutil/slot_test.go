package redisutil

import (
	"reflect"
	"testing"
)

func TestRemoveSlots(t *testing.T) {
	type args struct {
		slots        []Slot
		removedSlots []Slot
	}
	tests := []struct {
		name string
		args args
		want []Slot
	}{
		{
			name: "1",
			args: args{
				slots:        []Slot{2, 3, 4, 5, 6, 7, 8, 9, 10},
				removedSlots: []Slot{2, 10},
			},
			want: []Slot{3, 4, 5, 6, 7, 8, 9},
		},
		{
			name: "2",
			args: args{
				slots:        []Slot{2, 5},
				removedSlots: []Slot{2, 2, 3},
			},
			want: []Slot{5},
		},
		{
			name: "3",
			args: args{
				slots:        []Slot{0, 1, 3, 4},
				removedSlots: []Slot{0, 1, 3, 4},
			},
			want: []Slot{},
		},
		{
			name: "4",
			args: args{
				slots:        []Slot{},
				removedSlots: []Slot{2, 10},
			},
			want: []Slot{},
		},
		{
			name: "5",
			args: args{
				slots:        []Slot{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				removedSlots: []Slot{5},
			},
			want: []Slot{0, 1, 2, 3, 4, 6, 7, 8, 9, 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveSlots(tt.args.slots, tt.args.removedSlots); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveSlots() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveSlot(t *testing.T) {
	type args struct {
		slots       []Slot
		removedSlot Slot
	}
	tests := []struct {
		name string
		args args
		want []Slot
	}{
		{
			name: "1",
			args: args{
				slots:       []Slot{2, 3, 4, 5, 6, 7, 8, 9, 10},
				removedSlot: 2,
			},
			want: []Slot{3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			name: "2",
			args: args{
				slots:       []Slot{2, 5},
				removedSlot: 2,
			},
			want: []Slot{5},
		},
		{
			name: "3",
			args: args{
				slots:       []Slot{0, 1, 3, 4},
				removedSlot: 3,
			},
			want: []Slot{0, 1, 4},
		},
		{
			name: "4",
			args: args{
				slots:       []Slot{},
				removedSlot: 2,
			},
			want: []Slot{},
		},
		{
			name: "5",
			args: args{
				slots:       []Slot{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				removedSlot: 5,
			},
			want: []Slot{0, 1, 2, 3, 4, 6, 7, 8, 9, 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveSlot(tt.args.slots, tt.args.removedSlot); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveSlot() = %v, want %v", got, tt.want)
			}
		})
	}
}
