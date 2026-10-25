// Package cache provides explicit, bounded model-response caching. Namespaces
// must distinguish providers, models, and generation settings.
package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Store implementations must support concurrent Get and Set calls.
type Store interface {
	Get(string) ([]byte, bool, error)
	Set(string, []byte) error
}
type entry struct {
	key  string
	data []byte
}

// Memory is a bounded LRU store. Construct it with NewMemory.
type Memory struct {
	mu       sync.Mutex
	capacity int
	lru      *list.List
	items    map[string]*list.Element
}

func NewMemory(capacity int) (*Memory, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("cache: capacity must be positive")
	}
	return &Memory{capacity: capacity, lru: list.New(), items: map[string]*list.Element{}}, nil
}
func (m *Memory) Get(key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	m.lru.MoveToFront(e)
	return append([]byte(nil), e.Value.(entry).data...), true, nil
}
func (m *Memory) Set(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data = append([]byte(nil), data...)
	if e, ok := m.items[key]; ok {
		e.Value = entry{key, data}
		m.lru.MoveToFront(e)
		return nil
	}
	e := m.lru.PushFront(entry{key, data})
	m.items[key] = e
	if len(m.items) > m.capacity {
		old := m.lru.Back()
		delete(m.items, old.Value.(entry).key)
		m.lru.Remove(old)
	}
	return nil
}

// Disk stores private files with atomic replacement. It has no automatic size
// limit or expiry; callers own the cache directory and its lifecycle.
// Construct Disk with NewDisk; each file is private to the current OS user.
type Disk struct{ dir string }

func NewDisk(dir string) (*Disk, error) {
	if dir == "" {
		return nil, fmt.Errorf("cache: directory is required")
	}
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Disk{dir}, nil
}
func digest(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func (d *Disk) Get(key string) ([]byte, bool, error) {
	b, err := os.ReadFile(filepath.Join(d.dir, digest([]byte(key))+".json"))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return b, err == nil, err
}
func (d *Disk) Set(key string, data []byte) error {
	f, err := os.CreateTemp(d.dir, ".goeval-cache-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(d.dir, digest([]byte(key))+".json"))
}
func key(namespace, kind string, value any) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("cache: model namespace is required")
	}
	b, err := json.Marshal([]any{namespace, kind, value})
	if err != nil {
		return "", err
	}
	return digest(b), nil
}

// LLM does not hold a lock across provider calls. Simultaneous misses may issue
// duplicate requests; cached responses and store access are concurrency-safe.
func LLM(model goeval.LLM, store Store, namespace string) goeval.LLM {
	base := goeval.LLMFunc(func(ctx context.Context, p goeval.Prompt) (string, error) {
		if err := check.Context(ctx); err != nil {
			return "", err
		}
		if check.Nil(model) || check.Nil(store) {
			return "", fmt.Errorf("cache: model and store are required")
		}
		k, err := key(namespace, "llm", p)
		if err != nil {
			return "", err
		}
		b, ok, err := store.Get(k)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if ok {
			return string(b), nil
		}
		out, err := model.Generate(ctx, p)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := store.Set(k, []byte(out)); err != nil {
			return "", err
		}
		return out, nil
	})
	if probabilistic, ok := model.(goeval.LogProbLLM); ok {
		return &cachedLogProbModel{LLM: base, model: probabilistic, store: store, namespace: namespace}
	}
	return base
}

type cachedLogProbModel struct {
	goeval.LLM
	model     goeval.LogProbLLM
	store     Store
	namespace string
}

func (m *cachedLogProbModel) GenerateWithLogProbs(ctx context.Context, p goeval.Prompt) (goeval.Completion, error) {
	if err := check.Context(ctx); err != nil {
		return goeval.Completion{}, err
	}
	if check.Nil(m.store) || check.Nil(m.model) {
		return goeval.Completion{}, fmt.Errorf("cache: store is required")
	}
	p.LogProbs = true
	k, err := key(m.namespace, "llm.logprobs", p)
	if err != nil {
		return goeval.Completion{}, err
	}
	b, ok, err := m.store.Get(k)
	if err != nil {
		return goeval.Completion{}, err
	}
	if err := ctx.Err(); err != nil {
		return goeval.Completion{}, err
	}
	if ok {
		var out goeval.Completion
		err := json.Unmarshal(b, &out)
		return out, err
	}
	out, err := m.model.GenerateWithLogProbs(ctx, p)
	if err != nil {
		return goeval.Completion{}, err
	}
	if err := ctx.Err(); err != nil {
		return goeval.Completion{}, err
	}
	b, err = json.Marshal(out)
	if err != nil {
		return goeval.Completion{}, err
	}
	if err := m.store.Set(k, b); err != nil {
		return goeval.Completion{}, err
	}
	return out, nil
}
func Embeddings(model goeval.Embedder, store Store, namespace string) goeval.Embedder {
	return goeval.EmbedderFunc(func(ctx context.Context, texts []string) ([][]float64, error) {
		if err := check.Context(ctx); err != nil {
			return nil, err
		}
		if check.Nil(model) || check.Nil(store) {
			return nil, fmt.Errorf("cache: model and store are required")
		}
		k, err := key(namespace, "embeddings", texts)
		if err != nil {
			return nil, err
		}
		b, ok, err := store.Get(k)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ok {
			var out [][]float64
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			if err := check.Embeddings(out, len(texts)); err != nil {
				return nil, err
			}
			return out, nil
		}
		out, err := model.Embed(ctx, texts)
		if err != nil {
			return nil, err
		}
		if err := check.Embeddings(out, len(texts)); err != nil {
			return nil, err
		}
		b, err = json.Marshal(out)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := store.Set(k, b); err != nil {
			return nil, err
		}
		return out, nil
	})
}
