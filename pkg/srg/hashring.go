package srg

import (
	"fmt"
	"sort"

	"github.com/cespare/xxhash/v2"
)

type HashRing struct {
	nodes      []string
	vnodes     map[uint64]string
	sortedKeys []uint64
	vnodeCount int
}

func NewHashRing(nodes []string, vnodeCount int) *HashRing {
	r := &HashRing{
		nodes:      nodes,
		vnodes:     make(map[uint64]string),
		vnodeCount: vnodeCount,
	}

	for _, node := range nodes {
		for i := 0; i < vnodeCount; i++ {
			hash := xxhash.Sum64String(fmt.Sprintf("%s:%d", node, i))
			r.vnodes[hash] = node
			r.sortedKeys = append(r.sortedKeys, hash)
		}
	}

	sort.Slice(r.sortedKeys, func(i, j int) bool {
		return r.sortedKeys[i] < r.sortedKeys[j]
	})

	return r
}

func (r *HashRing) Get(key string) string {
	if len(r.nodes) == 0 {
		return ""
	}

	hash := xxhash.Sum64String(key)

	idx := sort.Search(len(r.sortedKeys), func(i int) bool {
		return r.sortedKeys[i] >= hash
	})

	if idx == len(r.sortedKeys) {
		idx = 0
	}

	return r.vnodes[r.sortedKeys[idx]]
}

func (r *HashRing) Nodes() []string {
	return r.nodes
}
