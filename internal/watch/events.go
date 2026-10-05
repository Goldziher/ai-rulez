package watch

import "github.com/fsnotify/fsnotify"

type watcherMessage struct {
	event fsnotify.Event
	err   error
}

// drainEvents keeps the backend free to acknowledge Add, Remove and Close
// while Run handles an event or Sync changes the watches. A bounded buffer can
// still deadlock when a Windows event batch fills it before acknowledging Add.
func (w *Watcher) drainEvents() {
	defer close(w.messages)
	events, errors := w.fs.Events, w.fs.Errors
	var pending []watcherMessage
	for events != nil || errors != nil || len(pending) > 0 {
		var output chan watcherMessage
		var next watcherMessage
		if len(pending) > 0 {
			output, next = w.messages, pending[0]
		}
		select {
		case <-w.stop:
			return
		case event, ok := <-events:
			if !ok {
				events = nil
			} else {
				pending = append(pending, watcherMessage{event: event})
			}
		case err, ok := <-errors:
			if !ok {
				errors = nil
			} else {
				pending = append(pending, watcherMessage{err: err})
			}
		case output <- next:
			pending[0] = watcherMessage{}
			pending = pending[1:]
		}
	}
}
