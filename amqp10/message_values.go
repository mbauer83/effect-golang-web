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

// brokerMessage is the library's message for one of ours.
func brokerMessage(message Message) (*broker.Message, error) {
	properties, err := Properties(message.Properties)
	if err != nil {
		return nil, err
	}

	sent := broker.NewMessage(message.Body)
	sent.ApplicationProperties = properties
	sent.Header = &broker.MessageHeader{Durable: message.Durability == Durable}
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

// deliveryOf is what arrived, in the universal representation.
//
// A message with no delivery tag cannot be settled, and settlement is the whole
// point of this package's consuming side -- so it is refused here rather than
// arriving as something the consumer will fail to acknowledge. That happens when
// the link was attached in the settle-on-send mode, which this package does not
// use.
func deliveryOf(message *broker.Message) (Delivery, error) {
	if len(message.DeliveryTag) == 0 {
		return Delivery{}, errNoTag
	}
	properties, err := ReadProperties(message.ApplicationProperties)
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{
		Body:        body(message),
		ContentType: text(contentType(message)),
		Subject:     text(subject(message)),
		Properties:  properties,
		Tag:         string(message.DeliveryTag),
		Attempts:    attempts(message),
	}, nil
}

// body is the message's data sections, joined.
//
// The protocol permits several, and they are one body: a sender that split a
// document across two is not sending two messages. Joining is what the
// specification says the body is.
func body(message *broker.Message) []byte {
	if len(message.Data) == 1 {
		return message.Data[0]
	}
	whole := []byte{}
	for _, section := range message.Data {
		whole = append(whole, section...)
	}
	return whole
}

func contentType(message *broker.Message) *string {
	if message.Properties == nil {
		return nil
	}
	return message.Properties.ContentType
}

func subject(message *broker.Message) *string {
	if message.Properties == nil {
		return nil
	}
	return message.Properties.Subject
}

func text(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

// attempts is the broker's count of previous deliveries.
func attempts(message *broker.Message) uint32 {
	if message.Header == nil {
		return 0
	}
	return message.Header.DeliveryCount
}
