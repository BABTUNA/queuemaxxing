package queue

// Journal durably records semantic queue mutations before they are applied to
// in-memory state.
type Journal interface {
	RecordEnqueue(Message) error
	RecordDequeue(messageID string) error
}

type noopJournal struct{}

func (noopJournal) RecordEnqueue(Message) error {
	return nil
}

func (noopJournal) RecordDequeue(string) error {
	return nil
}
