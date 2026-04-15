package queue

import (
	"fmt"

	"github.com/smarty/go-disruptor"
)

// DisruptorWrapper is a generic wrapper for the disruptor, with multiple consumer groups
type DisruptorWrapper[T any] struct {
	disruptor  disruptor.Disruptor
	bufferSize int64
	ringBuffer []T
	consumers  []Consumer[T]
}

func NewDisruptorWrapper[T any](bufferSize int64, consumers ...Consumer[T]) (*DisruptorWrapper[T], error) {
	if bufferSize&(bufferSize-1) != 0 {
		return nil, fmt.Errorf("bufferSize must be a power of 2")
	}

	ringBuffer := make([]T, bufferSize)

	wrapper := &DisruptorWrapper[T]{
		bufferSize: bufferSize,
		ringBuffer: ringBuffer,
		consumers:  consumers,
	}

	consumerGroup := make([]disruptor.Handler, len(consumers))
	for i, c := range consumers {
		consumerGroup[i] = &InternalConsumer[T]{buffer: ringBuffer, consumer: c}
	}

	//need consumer group
	myDisruptor, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(bufferSize)),
		disruptor.Options.NewHandlerGroup(consumerGroup...),
	)
	if err != nil {
		return nil, err
	}
	wrapper.disruptor = myDisruptor

	return wrapper, nil
}

func (w *DisruptorWrapper[T]) Start() {
	w.disruptor.Listen()
}

func (w *DisruptorWrapper[T]) Stop() {
	w.disruptor.Close()
}

func (w *DisruptorWrapper[T]) Publish(data ...T) {
	for _, d := range data {
		// Reserve a sequence for the message
		sequence := w.disruptor.Reserve(1)
		// Place the data in the Ring Buffer at the corresponding position
		w.ringBuffer[sequence%w.bufferSize] = d
		// Commit the sequence to notify consumers
		w.disruptor.Commit(sequence, sequence)
	}
}

type Consumer[T any] interface {
	Consume(lowerSequence, upperSequence int64, buffer []T)
}

// definition consumer: include buffer and consumer method
type InternalConsumer[T any] struct {
	buffer   []T
	consumer Consumer[T]
}

// implement disruptor handle method
func (c *InternalConsumer[T]) Handle(lowerSequence, upperSequence int64) {
	c.consumer.Consume(lowerSequence, upperSequence, c.buffer)
}
