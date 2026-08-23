package queue

import "errors"

var (
	ErrInvalidOrdering   = errors.New("ordering must be fifo or lifo")
	ErrEmptyBody         = errors.New("message body must not be empty")
	ErrInvalidUTF8       = errors.New("message body must be valid UTF-8")
	ErrBodyTooLarge      = errors.New("message body exceeds 256 KiB")
	ErrInvalidDelay      = errors.New("delay must be between 0 and 15 minutes")
	ErrNilJournal        = errors.New("journal must not be nil")
	ErrSequenceExhausted = errors.New("message sequence exhausted")
)
