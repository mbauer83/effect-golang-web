package amqp10

// One message across the library's own shape, in both directions.
//
// The body is a data section rather than an AMQP value section: a value section
// carries a typed AMQP value, which would mean encoding the schema's output
// twice -- once as JSON and once as an AMQP string -- and a receiver written in
// another language would read a quoted document. A data section is the bytes,
// which is what every other transport in this module sends.

import (
	broker "github.com/Azure/go-amqp"
)

// transferred is the library's message for one of ours.
func transferred(message Message) (*broker.Message, error) {
	properties, err := Properties(message.Properties)
	if err != nil {
		return nil, err
	}

	sent := broker.NewMessage(message.Body)
	sent.ApplicationProperties = properties
	sent.Header = &broker.MessageHeader{Durable: message.Durability == Lasting}
	if message.ContentType != "" || message.Subject != "" {
		sent.Properties = &broker.MessageProperties{}
		if message.ContentType != "" {
			sent.Properties.ContentType = &message.ContentType
		}
		if message.Subject != "" {
			sent.Properties.Subject = &message.Subject
		}
	}
	return sent, nil
}

// delivered is what arrived, in the universal representation.
//
// A message with no delivery tag cannot be settled, and settlement is the whole
// point of this package's consuming side -- so it is refused here rather than
// arriving as something the consumer will fail to acknowledge. That happens when
// the link was attached in the settle-on-send mode, which this package does not
// use.
func delivered(received *broker.Message) (Delivery, error) {
	if len(received.DeliveryTag) == 0 {
		return Delivery{}, errNoTag
	}
	properties, err := Named(received.ApplicationProperties)
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{
		Body:        body(received),
		ContentType: text(contentType(received)),
		Subject:     text(subject(received)),
		Properties:  properties,
		Tag:         string(received.DeliveryTag),
		Attempts:    attempts(received),
	}, nil
}

// body is the message's data sections, joined.
//
// The protocol permits several, and they are one body: a sender that split a
// document across two is not sending two messages. Joining is what the
// specification says the body is.
func body(received *broker.Message) []byte {
	if len(received.Data) == 1 {
		return received.Data[0]
	}
	whole := []byte{}
	for _, section := range received.Data {
		whole = append(whole, section...)
	}
	return whole
}

func contentType(received *broker.Message) *string {
	if received.Properties == nil {
		return nil
	}
	return received.Properties.ContentType
}

func subject(received *broker.Message) *string {
	if received.Properties == nil {
		return nil
	}
	return received.Properties.Subject
}

func text(held *string) string {
	if held == nil {
		return ""
	}
	return *held
}

// attempts is the broker's count of previous deliveries.
func attempts(received *broker.Message) uint32 {
	if received.Header == nil {
		return 0
	}
	return received.Header.DeliveryCount
}
