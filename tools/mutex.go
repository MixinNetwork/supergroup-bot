package tools

import "sync"

type Mutex struct {
	*sync.Mutex
	V map[string]interface{}
}

func NewMutex() *Mutex {
	m := new(Mutex)
	m.Mutex = new(sync.Mutex)
	m.V = make(map[string]interface{})
	return m
}

func (m *Mutex) Write(key string, v interface{}) {
	m.Lock()
	defer m.Unlock()
	m.V[key] = v
}

func (m *Mutex) Read(key string) interface{} {
	m.Lock()
	defer m.Unlock()
	return m.V[key]
}

func (m *Mutex) Delete(key string) {
	m.Lock()
	defer m.Unlock()
	delete(m.V, key)
}
