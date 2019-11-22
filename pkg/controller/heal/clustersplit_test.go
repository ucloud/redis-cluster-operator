package heal

import (
	"testing"

	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

func Test_buildClustersLists(t *testing.T) {
	// In the test below, we cannot directly use initialize redisutil.NodeSlice in redisutil.NodeInfos, this is a go vet issue: https://github.com/golang/go/issues/9171
	ip1 := redisutil.Nodes{{IP: "ip1", Port: "1234"}}
	ip2 := redisutil.Nodes{{IP: "ip2", Port: "1234"}}
	ip56 := redisutil.Nodes{{IP: "ip5", Port: "1234"}, {IP: "ip6", Port: "1234"}}
	ip64 := redisutil.Nodes{{IP: "ip6", Port: "1234"}, {IP: "ip4", Port: "1234"}}
	ip54 := redisutil.Nodes{{IP: "ip5", Port: "1234"}, {IP: "ip4", Port: "1234"}}
	// end of workaround
	testCases := []struct {
		input  *redisutil.ClusterInfos
		output []cluster
	}{ //several partilly different cannot happen, so not tested
		{ // empty
			input:  &redisutil.ClusterInfos{Infos: map[string]*redisutil.NodeInfos{}, Status: redisutil.ClusterInfosConsistent},
			output: []cluster{},
		},
		{ // one node
			input:  &redisutil.ClusterInfos{Infos: map[string]*redisutil.NodeInfos{"ip1:1234": {Node: &redisutil.Node{IP: "ip1", Port: "1234"}, Friends: redisutil.Nodes{}}}, Status: redisutil.ClusterInfosConsistent},
			output: []cluster{{"ip1:1234"}},
		},
		{ // no discrepency
			input: &redisutil.ClusterInfos{
				Infos: map[string]*redisutil.NodeInfos{
					"ip1:1234": {Node: &redisutil.Node{IP: "ip1", Port: "1234"}, Friends: ip2},
					"ip2:1234": {Node: &redisutil.Node{IP: "ip2", Port: "1234"}, Friends: ip1},
				},
				Status: redisutil.ClusterInfosConsistent,
			},
			output: []cluster{{"ip1:1234", "ip2:1234"}},
		},
		{ // several decorelated
			input: &redisutil.ClusterInfos{
				Infos: map[string]*redisutil.NodeInfos{
					"ip1:1234": {Node: &redisutil.Node{IP: "ip1", Port: "1234"}, Friends: ip2},
					"ip2:1234": {Node: &redisutil.Node{IP: "ip2", Port: "1234"}, Friends: ip1},
					"ip3:1234": {Node: &redisutil.Node{IP: "ip3", Port: "1234"}, Friends: redisutil.Nodes{}},
					"ip4:1234": {Node: &redisutil.Node{IP: "ip4", Port: "1234"}, Friends: ip56},
					"ip5:1234": {Node: &redisutil.Node{IP: "ip5", Port: "1234"}, Friends: ip64},
					"ip6:1234": {Node: &redisutil.Node{IP: "ip6", Port: "1234"}, Friends: ip54},
				},
				Status: redisutil.ClusterInfosInconsistent,
			},
			output: []cluster{{"ip1:1234", "ip2:1234"}, {"ip3:1234"}, {"ip4:1234", "ip5:1234", "ip6:1234"}},
		},
		{ // empty ignored
			input: &redisutil.ClusterInfos{
				Infos: map[string]*redisutil.NodeInfos{
					"ip1:1234": {Node: &redisutil.Node{IP: "ip1", Port: "1234"}, Friends: ip2},
					"ip2:1234": {Node: &redisutil.Node{IP: "ip2", Port: "1234"}, Friends: ip1},
					"ip3:1234": nil,
				},
				Status: redisutil.ClusterInfosInconsistent,
			},
			output: []cluster{{"ip1:1234", "ip2:1234"}},
		},
	}

	for i, tc := range testCases {
		output := buildClustersLists(tc.input)
		// because we work with map, order might not be conserved
		if !compareClusters(output, tc.output) {
			t.Errorf("[Case %d] Unexpected result for buildClustersLists, expected %v, got %v", i, tc.output, output)
		}
	}
}

func compareClusters(c1, c2 []cluster) bool {
	if len(c1) != len(c2) {
		return false
	}

	for _, c1elem := range c2 {
		found := false
		for _, c2elem := range c1 {
			if compareCluster(c1elem, c2elem) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func compareCluster(c1, c2 cluster) bool {
	if len(c1) != len(c2) {
		return false
	}
	for _, c1elem := range c2 {
		found := false
		for _, c2elem := range c1 {
			if c1elem == c2elem {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
