package main

type Storage interface {
	Init(*Config) error
	// Initialize storage. Perform all the checks. Maybe connect to database.

	SetQueue(string, []Message) error
	// Assign message queue to a key

	GetQueue(string) ([]Message, error)
	// Retrieve message queue by it's key

	DeleteQueue(string) error
	// Remove message queue assigned to a key

	AddMessage(string, Message) error
	// Append message to a queue retrieved by it's key

	GetKeys() ([]string, error)
	// Return list of all available keys
}

// Notifier is an optional interface a Storage may implement to support
// horizontal scaling. When a message is queued on one instance, Notify lets
// other instances know that a key has new messages so they can flush it to
// their own locally-connected clients. Subscribe registers the handler that
// reacts to those notifications.
type Notifier interface {
	Notify(key string) error
	Subscribe(handler func(key string))
}

// Storages maps an engine name to a constructor. Each Goctopus instance gets
// its own storage so instances (and tests) never share mutable state.
// Add your custom storage here:
var Storages = map[string]func() Storage{
	DEFAULT: func() Storage { return &MemoryStorage{} },
	MEMORY:  func() Storage { return &MemoryStorage{} },
	REDIS:   func() Storage { return &RedisStorage{} },
}
