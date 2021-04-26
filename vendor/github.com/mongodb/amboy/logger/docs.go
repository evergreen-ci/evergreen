// Package logger is a set of implementations to support amboy.Queue
// backed grip/send.Senders for asynchronous and (generally)
// non-blocking log message delivery.
//
// You can use amboy.Queue backed senders as an extension of an existing
// queue or use constructors that use the queue.NewLimitedSizeLocalQueue to
// deliver messages.
//
// These implementations do not guarantee delivery of log messages to
// senders in any particular order.
//
// These senders do not provide any batching or group message sending:
// messages are dispatched to queues immediately upon receipt. The
// grip/send.NewBufferedSender implementation has these properties.
//
// The multi-sender implementation provided by this method creates a
// single Job for every message. If you want to have a single Job for
// every message, use the grip/send.MakeMultiSender in combination
// with the single sender.
package logger

// this file is intentional documentation-only
