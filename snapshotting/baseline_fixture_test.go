package snapshotting

import (
	"errors"
	"sync"
)

// fakeArtifactStore is deliberately test-only. It gives snapshot tests a
// byte-preserving storage boundary before ArtifactStore is introduced in stage
// 2, without adding any production storage wiring.
type fakeArtifactStore struct {
	mu       sync.Mutex
	objects  map[string][]byte
	putError error
	getError error
}

func newFakeArtifactStore() *fakeArtifactStore {
	return &fakeArtifactStore{objects: make(map[string][]byte)}
}

func (s *fakeArtifactStore) put(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.putError != nil {
		return s.putError
	}
	s.objects[name] = append([]byte(nil), data...)
	return nil
}

func (s *fakeArtifactStore) get(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.getError != nil {
		return nil, s.getError
	}
	data, ok := s.objects[name]
	if !ok {
		return nil, errors.New("artifact does not exist")
	}
	return append([]byte(nil), data...), nil
}

// fixedMemoryFixture returns labelled pages used by snapshot, chunking, and
// provenance tests. The layout intentionally includes repeated and distinct
// content while remaining independent of a VM or an object store.
func fixedMemoryFixture(pageSize int) []byte {
	if pageSize <= 0 {
		panic("page size must be positive")
	}

	pages := []byte{'B', 'I', 'P', 'B'} // base, image, private, repeated base
	data := make([]byte, len(pages)*pageSize)
	for page, label := range pages {
		for offset := 0; offset < pageSize; offset++ {
			data[page*pageSize+offset] = label
		}
	}
	return data
}
