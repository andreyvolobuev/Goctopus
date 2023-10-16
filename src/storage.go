package main

type Storage interface {
	Init() error
	// Initialize storage. Perform all the checks. Maybe connect to database.

	SetQueue(string, []Message) error
	// Assign message queue to a key

	GetQueue(string) ([]Message, error)
	// Retrieve message queue by it's key

	DeleteQueue(string) error
	// Remove message queue assigned to a key

	AddMessage(string, Message) error
	// Append message to a queue retrieved by it's key
}

var Storages = map[string]Storage{
	"default": &MemoryStorage{},
	"memory":  &MemoryStorage{},
}
