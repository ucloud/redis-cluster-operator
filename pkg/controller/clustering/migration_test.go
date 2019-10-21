package clustering

import (
	"reflect"
	"testing"

	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

func TestDispatchSlotToMaster(t *testing.T) {
	masterRole := "master"

	redisNode1 := &redisutil.Node{ID: "1", Role: masterRole, IP: "1.1.1.1", Port: "1234", Slots: redisutil.BuildSlotSlice(0, 16383), PodName: "pod1"}
	redisNode2 := &redisutil.Node{ID: "2", Role: masterRole, IP: "1.1.1.2", Port: "1234", Slots: []redisutil.Slot{}, PodName: "pod2"}
	redisNode3 := &redisutil.Node{ID: "3", Role: masterRole, IP: "1.1.1.3", Port: "1234", Slots: []redisutil.Slot{}, PodName: "pod3"}
	redisNode4 := &redisutil.Node{ID: "4", Role: masterRole, IP: "1.1.1.4", Port: "1234", Slots: []redisutil.Slot{}, PodName: "pod4"}

	redisNode5 := &redisutil.Node{ID: "5", Role: masterRole, IP: "1.1.1.5", Port: "1234", Slots: redisutil.BuildSlotSlice(0, 5461), PodName: "pod1"}
	redisNode6 := &redisutil.Node{ID: "6", Role: masterRole, IP: "1.1.1.6", Port: "1234", Slots: redisutil.BuildSlotSlice(5462, 10923), PodName: "pod2"}
	redisNode7 := &redisutil.Node{ID: "7", Role: masterRole, IP: "1.1.1.7", Port: "1234", Slots: redisutil.BuildSlotSlice(10924, 16383), PodName: "pod4"}

	testCases := []struct {
		cluster   *redisutil.Cluster
		nodes     redisutil.Nodes
		nbMasters int32
		err       bool
	}{
		// append force copy, because DispatchSlotToMaster updates the slice
		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
				Nodes: map[string]*redisutil.Node{
					"1": redisNode1,
					"2": redisNode2,
					"3": redisNode3,
					"4": redisNode4,
				},
			},
			nodes: redisutil.Nodes{
				redisNode1,
				redisNode2,
				redisNode3,
				redisNode4,
			},
			nbMasters: 6, err: true,
		},
		// not enough master
		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
				Nodes: map[string]*redisutil.Node{
					"1": redisNode1,
					"2": redisNode2,
					"3": redisNode3,
					"4": redisNode4,
				},
			},
			nodes: redisutil.Nodes{
				redisNode1,
				redisNode2,
				redisNode3,
				redisNode4,
			},
			nbMasters: 2, err: false,
		}, // initial config

		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
				Nodes: map[string]*redisutil.Node{
					"1": redisNode1,
				},
			},
			nodes: redisutil.Nodes{
				redisNode1,
			},
			nbMasters: 1, err: false,
		}, // only one node

		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
				Nodes: map[string]*redisutil.Node{
					"2": redisNode2,
				},
			},
			nodes: redisutil.Nodes{
				redisNode2,
			},
			nbMasters: 1, err: false,
		}, // only one node with no slots
		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
			},
			nodes: redisutil.Nodes{}, nbMasters: 0, err: false,
		}, // empty
		{
			cluster: &redisutil.Cluster{
				Name:      "clustertest",
				Namespace: "default",
				Nodes: map[string]*redisutil.Node{
					"2": redisNode2,
					"3": redisNode3,
					"5": redisNode5,
					"6": redisNode6,
					"7": redisNode7,
				},
			},
			nodes: redisutil.Nodes{
				redisNode5,
				redisNode6,
				redisNode7,
				redisNode2,
				redisNode3,
			},
			nbMasters: 4, err: false,
		},
	}

	for i, tc := range testCases {
		if i != 5 {
			continue
		}
		n, c, a, err := DispatchMasters(tc.cluster, tc.nodes, tc.nbMasters)
		if (err != nil) != tc.err {
			t.Errorf("[case: %d] Unexpected error status, expected error to be %t, got '%v'", i, tc.err, err)
		}
		t.Logf("newMasters: %v\n", n)
		t.Logf("curMasters: %v\n", c)
		t.Logf("allMasters: %v\n", a)
	}
}

func Test_buildSlotsByNode(t *testing.T) {
	redis1 := &redisutil.Node{ID: "redis1", Slots: []redisutil.Slot{0, 1, 2, 3, 4, 5, 6, 7, 8}}
	redis2 := &redisutil.Node{ID: "redis2", Slots: []redisutil.Slot{}}
	redis3 := &redisutil.Node{ID: "redis3", Slots: []redisutil.Slot{}}

	redis4 := &redisutil.Node{ID: "redis4", Slots: []redisutil.Slot{0, 1, 2, 3, 4}}
	redis5 := &redisutil.Node{ID: "redis5", Slots: []redisutil.Slot{5, 6, 7, 8}}
	redis6 := &redisutil.Node{ID: "redis6", Slots: []redisutil.Slot{}}
	//redis7 := &redisutil.Node{ID: "redis7", Slots: []redisutil.Slot{0, 1, 2, 3}}
	//redis8 := &redisutil.Node{ID: "redis8", Slots: []redisutil.Slot{4, 5, 6, 7}}
	//redis9 := &redisutil.Node{ID: "redis9", Slots: []redisutil.Slot{8, 9, 10}}

	redis10 := &redisutil.Node{ID: "redis10", Slots: redisutil.BuildSlotSlice(0, 5461)}
	redis11 := &redisutil.Node{ID: "redis11", Slots: redisutil.BuildSlotSlice(5462, 10923)}
	redis12 := &redisutil.Node{ID: "redis12", Slots: redisutil.BuildSlotSlice(10924, 16383)}

	type args struct {
		newMasterNodes redisutil.Nodes
		oldMasterNodes redisutil.Nodes
		allMasterNodes redisutil.Nodes
		nbSlots        int
		run            bool
	}
	tests := []struct {
		name string
		args args
		want map[string]int
	}{
		{
			name: "2 new nodes",
			args: args{
				newMasterNodes: redisutil.Nodes{redis1, redis2, redis3},
				oldMasterNodes: redisutil.Nodes{redis1},
				allMasterNodes: redisutil.Nodes{redis1, redis2, redis3},
				nbSlots:        9,
			},
			want: map[string]int{
				redis2.ID: 3,
				redis3.ID: 3,
			},
		},
		{
			name: "1 new node",
			args: args{
				newMasterNodes: redisutil.Nodes{redis1, redis2},
				oldMasterNodes: redisutil.Nodes{redis1},
				allMasterNodes: redisutil.Nodes{redis1, redis2},
				nbSlots:        9,
			},
			want: map[string]int{
				redis2.ID: 4,
			},
		},
		{
			name: "2 new nodes, one removed",
			args: args{
				newMasterNodes: redisutil.Nodes{redis4, redis2, redis3},
				oldMasterNodes: redisutil.Nodes{redis4, redis5},
				allMasterNodes: redisutil.Nodes{redis4, redis2, redis3, redis5},
				nbSlots:        9,
			},
			want: map[string]int{
				redis2.ID: 3,
				redis3.ID: 3,
			},
		},
		{
			name: "3 new nodes, 0 removed",
			args: args{
				newMasterNodes: redisutil.Nodes{redis2, redis3, redis6},
				oldMasterNodes: redisutil.Nodes{},
				allMasterNodes: redisutil.Nodes{redis2, redis3, redis6},
				nbSlots:        11,
			},
			want: map[string]int{
				redis2.ID: 4,
				redis3.ID: 4,
				redis6.ID: 3,
			},
		},
		{
			name: "4 new nodes, 3 removed",
			args: args{
				newMasterNodes: redisutil.Nodes{redis10, redis11, redis12, redis2},
				oldMasterNodes: redisutil.Nodes{redis10, redis11, redis12},
				allMasterNodes: redisutil.Nodes{redis10, redis11, redis12, redis2, redis3},
				nbSlots:        16384,
				run:            true,
			},
			want: map[string]int{
				redis11.ID: 1,
				redis12.ID: 1,
				redis2.ID:  4094,
			},
		},
	}
	for _, tt := range tests {
		if !tt.args.run {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			got := buildSlotsByNode(tt.args.newMasterNodes, tt.args.oldMasterNodes, tt.args.allMasterNodes, tt.args.nbSlots)
			gotSlotByNodeID := make(map[string]int)
			for id, slots := range got {
				t.Logf("id:%s, len:%d, slots:%d\n", id, len(slots), slots)
				gotSlotByNodeID[id] = len(slots)
			}
			if !reflect.DeepEqual(gotSlotByNodeID, tt.want) {
				t.Errorf("buildSlotsByNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_feedMigInfo(t *testing.T) {
	redis1 := &redisutil.Node{ID: "redis1", Slots: []redisutil.Slot{0, 1, 2, 3, 4, 5, 6, 7, 8}}
	redis2 := &redisutil.Node{ID: "redis2", Slots: []redisutil.Slot{}}
	redis3 := &redisutil.Node{ID: "redis3", Slots: []redisutil.Slot{}}

	type args struct {
		newMasterNodes redisutil.Nodes
		oldMasterNodes redisutil.Nodes
		allMasterNodes redisutil.Nodes
		nbSlots        int
	}
	tests := []struct {
		name       string
		args       args
		wantMapOut mapSlotByMigInfo
	}{
		{
			name: "basic usecase",
			args: args{
				newMasterNodes: redisutil.Nodes{redis1, redis2, redis3},
				oldMasterNodes: redisutil.Nodes{redis1},
				allMasterNodes: redisutil.Nodes{redis1, redis2, redis3},
				nbSlots:        9,
			},
			wantMapOut: mapSlotByMigInfo{
				migrationInfo{From: redis1, To: redis2}: []redisutil.Slot{3, 4, 5},
				migrationInfo{From: redis1, To: redis3}: []redisutil.Slot{6, 7, 8},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotMapOut, _ := feedMigInfo(tt.args.newMasterNodes, tt.args.oldMasterNodes, tt.args.allMasterNodes, tt.args.nbSlots); !reflect.DeepEqual(gotMapOut, tt.wantMapOut) {
				t.Errorf("feedMigInfo() = %v, want %v", gotMapOut, tt.wantMapOut)
			}
		})
	}
}
