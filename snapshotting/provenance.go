package snapshotting

// The provenance working-set format deliberately keeps the decision to share
// separate from the content hash.  A hash is an address *after* a policy has
// approved the page's provenance; it is never evidence that a page is safe to
// share.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/vhive-serverless/vhive/memory/manager"
)

const (
	provenanceWorkingSetVersion = 1
	provenanceManifestArtifact  = "working-set-v1.json"
	privateContentArtifact      = "working-set-private-v1.content"
	privateIndexArtifact        = "working-set-private-v1.index"
)

type ContentScope string

const (
	PrivateScope    ContentScope = "private"
	BaseRootfsScope ContentScope = "base-rootfs"
	ImageScope      ContentScope = "image"
)

// WorkingSetPage is a recorded guest page. PFN is its byte offset in the
// guest-memory file (the name is retained for compatibility with the paper).
type WorkingSetPage struct {
	PFN   uint64
	Bytes []byte
}

type ProvenancePolicy interface {
	Classify(page WorkingSetPage, image string) ContentScope
}

// AllPrivateProvenance is the safe default and preserves the old behaviour.
type AllPrivateProvenance struct{}

func (AllPrivateProvenance) Classify(WorkingSetPage, string) ContentScope { return PrivateScope }

// ContentHashProvenance implements SplitSnap-style offline classification.
// It compares recorded guest pages with trusted content sources. Base snapshot
// and rootfs pages are globally shareable; image pages are shareable only when
// their bytes occur in the image that produced the snapshot. Shared IDs remain
// canonical page hashes, so identical pages in two approved images reuse the
// same content address.
type ContentHashProvenance struct {
	mu     sync.RWMutex
	base   map[string]struct{}
	images map[string]map[string]struct{}
}

// NewContentHashProvenance builds the known-safe registry from page-aligned
// files. baseRootfsFiles normally contains the base snapshot memory file and
// bin/default-rootfs.img. imageFiles is keyed by the immutable image identity
// recorded in the snapshot descriptor.
func NewContentHashProvenance(pageSize uint64, baseRootfsFiles []string, imageFiles map[string][]string) (*ContentHashProvenance, error) {
	if pageSize == 0 {
		return nil, fmt.Errorf("provenance page size must be positive")
	}
	p := &ContentHashProvenance{base: make(map[string]struct{}), images: make(map[string]map[string]struct{})}
	if err := p.AddBaseRootfsFiles(pageSize, baseRootfsFiles...); err != nil {
		return nil, err
	}
	for image, files := range imageFiles {
		if err := p.AddImageFiles(pageSize, image, files...); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *ContentHashProvenance) AddBaseRootfsFiles(pageSize uint64, files ...string) error {
	hashes, err := pageHashesFromFiles(pageSize, files)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for hash := range hashes {
		p.base[hash] = struct{}{}
	}
	return nil
}

func (p *ContentHashProvenance) AddImageFiles(pageSize uint64, image string, files ...string) error {
	if image == "" {
		return fmt.Errorf("image identity is required")
	}
	hashes, err := pageHashesFromFiles(pageSize, files)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.images[image] == nil {
		p.images[image] = make(map[string]struct{})
	}
	for hash := range hashes {
		p.images[image][hash] = struct{}{}
	}
	return nil
}

func (p *ContentHashProvenance) Classify(page WorkingSetPage, image string) ContentScope {
	hash := pageHash(page.Bytes)
	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, ok := p.base[hash]; ok {
		return BaseRootfsScope
	}
	if _, ok := p.images[image][hash]; ok {
		return ImageScope
	}
	return PrivateScope
}

func pageHashesFromFiles(pageSize uint64, files []string) (map[string]struct{}, error) {
	hashes := make(map[string]struct{})
	for _, file := range files {
		if file == "" {
			continue
		}
		clean := filepath.Clean(file)
		info, err := os.Stat(clean)
		if err != nil {
			return nil, fmt.Errorf("stat provenance source %s: %w", clean, err)
		}
		if info.IsDir() {
			err = filepath.Walk(clean, func(path string, entry os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.Mode().IsRegular() {
					return nil
				}
				return addFilePageHashes(pageSize, hashes, path)
			})
		} else {
			err = addFilePageHashes(pageSize, hashes, clean)
		}
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

func addFilePageHashes(pageSize uint64, hashes map[string]struct{}, path string) error {
	if pageSize > uint64(int(^uint(0)>>1)) {
		return fmt.Errorf("provenance page size too large: %d", pageSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open provenance source %s: %w", path, err)
	}
	defer f.Close()
	buf := make([]byte, int(pageSize))
	for {
		n, readErr := io.ReadFull(f, buf)
		if n == len(buf) {
			hashes[pageHash(buf)] = struct{}{}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read provenance source %s: %w", path, readErr)
		}
	}
}

// FixtureProvenance maps recorded PFNs to explicit provenance. It is useful
// for deterministic tests and for adapters which already have provenance maps.
// Entries absent from the map are private.
type FixtureProvenance map[uint64]ContentScope

func (p FixtureProvenance) Classify(page WorkingSetPage, _ string) ContentScope {
	switch p[page.PFN] {
	case BaseRootfsScope, ImageScope:
		return p[page.PFN]
	default:
		return PrivateScope
	}
}

type workingSetSourceRef struct {
	Identity string `json:"identity,omitempty"`
	Content  string `json:"content,omitempty"`
	Index    string `json:"index,omitempty"`
}

type workingSetPageRef struct {
	PFN   uint64       `json:"pfn"`
	Hash  string       `json:"hash"`
	Scope ContentScope `json:"scope"`
}

// WorkingSetManifest is the versioned reference published only after all
// referenced source files are durable. Shared sources use hash-to-offset
// indexes; private sources use PFN-to-offset indexes.
type WorkingSetManifest struct {
	Version  int                 `json:"version"`
	PageSize uint64              `json:"pageSize"`
	Base     workingSetSourceRef `json:"base,omitempty"`
	Image    workingSetSourceRef `json:"image,omitempty"`
	Private  workingSetSourceRef `json:"private,omitempty"`
	Pages    []workingSetPageRef `json:"pages"`
}

type contentIndexEntry struct {
	Hash   string `json:"hash"`
	Offset uint64 `json:"offset"`
	Length uint64 `json:"length"`
}

type privateIndexEntry struct {
	PFN    uint64 `json:"pfn"`
	Hash   string `json:"hash"`
	Offset uint64 `json:"offset"`
	Length uint64 `json:"length"`
}

// WorkingSetRepository publishes and loads provenance-partitioned working
// sets. baseIdentity must name an immutable base/rootfs version; image is the
// stable image identity from the snapshot descriptor.
type WorkingSetRepository struct {
	store        ArtifactStore
	policy       ProvenancePolicy
	baseIdentity string
}

func NewWorkingSetRepository(store ArtifactStore, policy ProvenancePolicy, baseIdentity string) (*WorkingSetRepository, error) {
	if store == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	if policy == nil {
		policy = AllPrivateProvenance{}
	}
	if baseIdentity == "" {
		baseIdentity = "default"
	}
	return &WorkingSetRepository{store: store, policy: policy, baseIdentity: baseIdentity}, nil
}

// Publish writes private revision content and immutable shared base/image
// content, then publishes the manifest last. It intentionally has no
// compression path: the format stores decoded page bytes.
func (r *WorkingSetRepository) Publish(ctx context.Context, revision, image string, pageSize uint64, pages []WorkingSetPage) (*WorkingSetManifest, error) {
	if pageSize == 0 {
		return nil, fmt.Errorf("working-set page size must be positive")
	}
	if err := validateKeyPart("revision", revision); err != nil {
		return nil, err
	}
	seen := map[uint64]struct{}{}
	groups := map[ContentScope][]WorkingSetPage{PrivateScope: nil, BaseRootfsScope: nil, ImageScope: nil}
	refs := make([]workingSetPageRef, 0, len(pages))
	for _, page := range pages {
		if _, ok := seen[page.PFN]; ok {
			return nil, fmt.Errorf("duplicate working-set PFN %#x", page.PFN)
		}
		seen[page.PFN] = struct{}{}
		if uint64(len(page.Bytes)) != pageSize {
			return nil, fmt.Errorf("working-set page %#x has size %d, want %d", page.PFN, len(page.Bytes), pageSize)
		}
		scope := r.policy.Classify(page, image)
		if scope != BaseRootfsScope && scope != ImageScope {
			scope = PrivateScope
		}
		hash := scopedPageID(scope, revision, page.Bytes)
		groups[scope] = append(groups[scope], page)
		refs = append(refs, workingSetPageRef{PFN: page.PFN, Hash: hash, Scope: scope})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].PFN < refs[j].PFN })
	manifest := &WorkingSetManifest{Version: provenanceWorkingSetVersion, PageSize: pageSize, Pages: refs}
	var err error
	if manifest.Private, err = r.putPrivate(ctx, revision, groups[PrivateScope]); err != nil {
		return nil, err
	}
	if manifest.Base, err = r.putShared(ctx, "working-set-base-v1", r.baseIdentity, groups[BaseRootfsScope]); err != nil {
		return nil, err
	}
	if manifest.Image, err = r.putShared(ctx, "working-set-image-v1", image, groups[ImageScope]); err != nil {
		return nil, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	key, err := RevisionArtifactKey(revision, provenanceManifestArtifact)
	if err != nil {
		return nil, err
	}
	if err := r.store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, fmt.Errorf("publish working-set manifest: %w", err)
	}
	return manifest, nil
}

func (r *WorkingSetRepository) putPrivate(ctx context.Context, revision string, pages []WorkingSetPage) (workingSetSourceRef, error) {
	sort.Slice(pages, func(i, j int) bool { return pages[i].PFN < pages[j].PFN })
	capacity := 0
	if len(pages) != 0 {
		capacity = len(pages) * len(pages[0].Bytes)
	}
	content := make([]byte, 0, capacity)
	index := make([]privateIndexEntry, 0, len(pages))
	for _, p := range pages {
		off := uint64(len(content))
		content = append(content, p.Bytes...)
		index = append(index, privateIndexEntry{p.PFN, privatePageID(revision, p.Bytes), off, uint64(len(p.Bytes))})
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		return workingSetSourceRef{}, err
	}
	contentKey, _ := RevisionArtifactKey(revision, privateContentArtifact)
	indexKey, _ := RevisionArtifactKey(revision, privateIndexArtifact)
	if err := r.store.Put(ctx, contentKey, bytes.NewReader(content), int64(len(content))); err != nil {
		return workingSetSourceRef{}, err
	}
	if err := r.store.Put(ctx, indexKey, bytes.NewReader(indexData), int64(len(indexData))); err != nil {
		return workingSetSourceRef{}, err
	}
	storedContent, err := readArtifact(ctx, r.store, contentKey)
	if err != nil {
		return workingSetSourceRef{}, err
	}
	storedIndex, err := readArtifact(ctx, r.store, indexKey)
	if err != nil {
		return workingSetSourceRef{}, err
	}
	if !bytes.Equal(storedContent, content) || !bytes.Equal(storedIndex, indexData) {
		return workingSetSourceRef{}, fmt.Errorf("private working-set artifacts failed validation")
	}
	return workingSetSourceRef{Identity: revision, Content: string(contentKey), Index: string(indexKey)}, nil
}

func (r *WorkingSetRepository) putShared(ctx context.Context, kind, identity string, pages []WorkingSetPage) (workingSetSourceRef, error) {
	if len(pages) == 0 {
		return workingSetSourceRef{}, nil
	}
	// One canonical copy for each hash makes content and its hash index compact.
	unique := map[string][]byte{}
	for _, p := range pages {
		unique[pageHash(p.Bytes)] = p.Bytes
	}
	hashes := make([]string, 0, len(unique))
	for h := range unique {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	content := make([]byte, 0)
	index := make([]contentIndexEntry, 0, len(hashes))
	for _, h := range hashes {
		off := uint64(len(content))
		data := unique[h]
		content = append(content, data...)
		index = append(index, contentIndexEntry{h, off, uint64(len(data))})
	}
	// Registry image names contain path separators. A digest is both a stable
	// object-key identity and avoids treating storage-key syntax as provenance.
	// Include the encoded content version as well: a new observed working set
	// is copy-on-write and old manifests keep their immutable source intact.
	storageIdentity := identityHash(identity) + "-" + identityHash(string(content))
	contentKey, err := SharedArtifactKey(kind, storageIdentity+".content")
	if err != nil {
		return workingSetSourceRef{}, err
	}
	indexKey, err := SharedArtifactKey(kind, storageIdentity+".index")
	if err != nil {
		return workingSetSourceRef{}, err
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		return workingSetSourceRef{}, err
	}
	if err := putImmutable(ctx, r.store, contentKey, content); err != nil {
		return workingSetSourceRef{}, err
	}
	if err := putImmutable(ctx, r.store, indexKey, indexData); err != nil {
		return workingSetSourceRef{}, err
	}
	return workingSetSourceRef{Identity: identity, Content: string(contentKey), Index: string(indexKey)}, nil
}

func putImmutable(ctx context.Context, store ArtifactStore, key ArtifactKey, data []byte) error {
	reader, err := store.Get(ctx, key)
	if err == nil {
		existing, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("immutable shared working-set artifact %q already differs", key)
		}
		return nil
	}
	if !isArtifactNotFound(err) {
		return err
	}
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return err
	}
	stored, err := readArtifact(ctx, store, key)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, data) {
		return fmt.Errorf("immutable shared working-set artifact %q failed validation", key)
	}
	return nil
}

func pageHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func privatePageID(revision string, data []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("private-working-set-v1\x00"))
	_, _ = hash.Write([]byte(revision))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

func scopedPageID(scope ContentScope, revision string, data []byte) string {
	if scope == PrivateScope {
		return privatePageID(revision, data)
	}
	return pageHash(data)
}

func identityHash(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

// PublishFiles adapts the existing coalesced working-set file to the new
// format. The trace defines the page order, so no provenance is inferred from
// content equality.
func (r *WorkingSetRepository) PublishFiles(ctx context.Context, revision, image, tracePath, pagesPath string) (*WorkingSetManifest, error) {
	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		return nil, err
	}
	var trace struct {
		Version  int      `json:"version"`
		PageSize uint64   `json:"page_size"`
		Offsets  []uint64 `json:"offsets"`
	}
	if err := json.Unmarshal(traceData, &trace); err != nil {
		return nil, err
	}
	if trace.Version != 1 || trace.PageSize == 0 {
		return nil, fmt.Errorf("invalid working-set trace")
	}
	data, err := os.ReadFile(pagesPath)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) != uint64(len(trace.Offsets))*trace.PageSize {
		return nil, fmt.Errorf("working-set content size %d does not match trace", len(data))
	}
	pages := make([]WorkingSetPage, len(trace.Offsets))
	for i, pfn := range trace.Offsets {
		start := uint64(i) * trace.PageSize
		pages[i] = WorkingSetPage{PFN: pfn, Bytes: append([]byte(nil), data[start:start+trace.PageSize]...)}
	}
	return r.Publish(ctx, revision, image, trace.PageSize, pages)
}

func (r *WorkingSetRepository) Load(ctx context.Context, revision string) (*WorkingSetManifest, error) {
	key, err := RevisionArtifactKey(revision, provenanceManifestArtifact)
	if err != nil {
		return nil, err
	}
	reader, err := r.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var manifest WorkingSetManifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return nil, err
	}
	if err := validateWorkingSetManifest(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateWorkingSetManifest(m *WorkingSetManifest) error {
	if m.Version != provenanceWorkingSetVersion || m.PageSize == 0 {
		return fmt.Errorf("unsupported working-set manifest")
	}
	seen := map[uint64]struct{}{}
	for _, p := range m.Pages {
		if _, ok := seen[p.PFN]; ok {
			return fmt.Errorf("duplicate manifest PFN %#x", p.PFN)
		}
		seen[p.PFN] = struct{}{}
		if p.Hash == "" {
			return fmt.Errorf("empty page hash")
		}
	}
	return nil
}

// NewProvenanceWorkingSetPageSource overlays a provenance working set on an
// ordinary memory source. Unknown pages (and non-page-aligned reads) retain
// the normal recipe/local-file behaviour.
func NewProvenanceWorkingSetPageSource(ctx context.Context, store ArtifactStore, revision string, fallback manager.PageSource) (manager.PageSource, error) {
	workingSet, err := LoadProvenanceWorkingSet(ctx, store, revision)
	if err != nil {
		return nil, err
	}
	return workingSet.NewPageSource(ctx, store, "", fallback)
}

// ProvenanceWorkingSet contains only immutable shared base/image working-set
// data. Private pages are deliberately loaded into each page source and are
// never retained in this restore cache.
type ProvenanceWorkingSet struct {
	pageSize   uint64
	pages      map[uint64]workingSetPageRef
	privateRef workingSetSourceRef
	shared     map[ContentScope]map[string][]byte
}

// LoadProvenanceWorkingSet downloads and validates a revision's shared
// provenance working-set data. Callers may retain the result because it
// intentionally excludes private page contents.
func LoadProvenanceWorkingSet(ctx context.Context, store ArtifactStore, revision string) (*ProvenanceWorkingSet, error) {
	repo, err := NewWorkingSetRepository(store, nil, "default")
	if err != nil {
		return nil, err
	}
	manifest, err := repo.Load(ctx, revision)
	if err != nil {
		return nil, err
	}
	return newProvenanceWorkingSet(ctx, store, manifest)
}

// NewPageSource loads a fresh private working set for one restore and combines
// it with the cached shared data and a per-restore fallback memory source.
func (s *ProvenanceWorkingSet) NewPageSource(ctx context.Context, store ArtifactStore, privateCacheDir string, fallback manager.PageSource) (manager.PageSource, error) {
	private := make(map[uint64][]byte)
	if err := loadPrivateFromCacheOrStore(ctx, store, privateCacheDir, s.privateRef, private); err != nil {
		return nil, err
	}
	return &provenancePageSource{ProvenanceWorkingSet: s, private: private, fallback: fallback}, nil
}

type provenancePageSource struct {
	*ProvenanceWorkingSet
	private  map[uint64][]byte
	fallback manager.PageSource
}

func newProvenanceWorkingSet(ctx context.Context, store ArtifactStore, m *WorkingSetManifest) (*ProvenanceWorkingSet, error) {
	s := &ProvenanceWorkingSet{pageSize: m.PageSize, pages: map[uint64]workingSetPageRef{}, privateRef: m.Private, shared: map[ContentScope]map[string][]byte{}}
	for _, p := range m.Pages {
		s.pages[p.PFN] = p
	}

	// The two shared sources are immutable and independent. Fetch them in
	// parallel; private data is intentionally loaded per page source below.
	type sharedResult struct {
		scope ContentScope
		pages map[string][]byte
		err   error
	}
	sharedCh := make(chan sharedResult, 2)
	for _, source := range []struct {
		scope ContentScope
		ref   workingSetSourceRef
	}{
		{scope: BaseRootfsScope, ref: m.Base},
		{scope: ImageScope, ref: m.Image},
	} {
		go func(scope ContentScope, ref workingSetSourceRef) {
			pages, err := loadShared(ctx, store, ref)
			sharedCh <- sharedResult{scope: scope, pages: pages, err: err}
		}(source.scope, source.ref)
	}

	shared := map[ContentScope]sharedResult{}
	for range 2 {
		result := <-sharedCh
		shared[result.scope] = result
	}
	if result := shared[BaseRootfsScope]; result.err != nil {
		return nil, result.err
	}
	if result := shared[ImageScope]; result.err != nil {
		return nil, result.err
	}
	for _, scope := range []ContentScope{BaseRootfsScope, ImageScope} {
		result := shared[scope]
		if len(result.pages) != 0 {
			s.shared[result.scope] = result.pages
		}
	}
	return s, nil
}

func readArtifactPair(ctx context.Context, store ArtifactStore, contentKey, indexKey ArtifactKey) ([]byte, []byte, error) {
	type result struct {
		content bool
		data    []byte
		err     error
	}
	results := make(chan result, 2)
	go func() {
		data, err := readArtifact(ctx, store, contentKey)
		results <- result{content: true, data: data, err: err}
	}()
	go func() {
		data, err := readArtifact(ctx, store, indexKey)
		results <- result{data: data, err: err}
	}()

	first, second := <-results, <-results
	if first.err != nil {
		return nil, nil, first.err
	}
	if second.err != nil {
		return nil, nil, second.err
	}
	if first.content {
		return first.data, second.data, nil
	}
	return second.data, first.data, nil
}

func loadPrivate(ctx context.Context, store ArtifactStore, ref workingSetSourceRef, out map[uint64][]byte) error {
	if ref.Content == "" {
		return nil
	}
	content, raw, err := readArtifactPair(ctx, store, ArtifactKey(ref.Content), ArtifactKey(ref.Index))
	if err != nil {
		return err
	}
	return decodePrivate(ref, content, raw, out)
}

// loadPrivateFromCacheOrStore uses the local snapshot copy only when the
// caller explicitly selected snapshot retention. A missing local copy falls
// back to the remote store so a newly published snapshot remains restorable.
func loadPrivateFromCacheOrStore(ctx context.Context, store ArtifactStore, privateCacheDir string, ref workingSetSourceRef, out map[uint64][]byte) error {
	if ref.Content == "" {
		return nil
	}
	if privateCacheDir != "" {
		content, contentErr := os.ReadFile(filepath.Join(privateCacheDir, privateContentArtifact))
		raw, indexErr := os.ReadFile(filepath.Join(privateCacheDir, privateIndexArtifact))
		if contentErr == nil && indexErr == nil {
			return decodePrivate(ref, content, raw, out)
		}
		if (contentErr != nil && !os.IsNotExist(contentErr)) || (indexErr != nil && !os.IsNotExist(indexErr)) {
			if contentErr != nil {
				return fmt.Errorf("read cached private working-set content: %w", contentErr)
			}
			return fmt.Errorf("read cached private working-set index: %w", indexErr)
		}
	}
	return loadPrivate(ctx, store, ref, out)
}

func decodePrivate(ref workingSetSourceRef, content, raw []byte, out map[uint64][]byte) error {
	var index []privateIndexEntry
	if err := json.Unmarshal(raw, &index); err != nil {
		return err
	}
	for _, e := range index {
		if e.Offset > uint64(len(content)) || e.Length > uint64(len(content))-e.Offset || privatePageID(ref.Identity, content[e.Offset:e.Offset+e.Length]) != e.Hash {
			return fmt.Errorf("invalid private working-set index")
		}
		out[e.PFN] = append([]byte(nil), content[e.Offset:e.Offset+e.Length]...)
	}
	return nil
}
func loadShared(ctx context.Context, store ArtifactStore, ref workingSetSourceRef) (map[string][]byte, error) {
	out := map[string][]byte{}
	if ref.Content == "" {
		return out, nil
	}
	content, raw, err := readArtifactPair(ctx, store, ArtifactKey(ref.Content), ArtifactKey(ref.Index))
	if err != nil {
		return nil, err
	}
	var index []contentIndexEntry
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	for _, e := range index {
		if e.Offset > uint64(len(content)) || e.Length > uint64(len(content))-e.Offset || pageHash(content[e.Offset:e.Offset+e.Length]) != e.Hash {
			return nil, fmt.Errorf("invalid shared working-set index")
		}
		out[e.Hash] = append([]byte(nil), content[e.Offset:e.Offset+e.Length]...)
	}
	return out, nil
}
func readArtifact(ctx context.Context, store ArtifactStore, key ArtifactKey) ([]byte, error) {
	reader, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
func (s *provenancePageSource) ReadAt(ctx context.Context, offset, length uint64) (manager.PageData, error) {
	if length == s.pageSize {
		if ref, ok := s.pages[offset]; ok {
			var data []byte
			if ref.Scope == PrivateScope {
				data = s.private[offset]
			} else {
				data = s.shared[ref.Scope][ref.Hash]
			}
			if uint64(len(data)) != length {
				return manager.PageData{}, fmt.Errorf("working-set page %#x is unavailable", offset)
			}
			zero := bytes.Equal(data, make([]byte, len(data)))
			return manager.PageData{Bytes: append([]byte(nil), data...), Zero: zero}, nil
		}
	}
	if s.fallback == nil {
		return manager.PageData{}, fmt.Errorf("working-set page %#x is unavailable", offset)
	}
	return s.fallback.ReadAt(ctx, offset, length)
}
func (s *provenancePageSource) Close() error {
	if s.fallback != nil {
		return s.fallback.Close()
	}
	return nil
}
func (s *provenancePageSource) DownloadedChunkCount() uint64 {
	if c, ok := s.fallback.(manager.ChunkDownloadCounter); ok {
		return c.DownloadedChunkCount()
	}
	return 0
}
