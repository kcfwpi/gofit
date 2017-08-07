package gofit

import (
	"encoding/binary"
	"errors"
	"io"
	"time"
)

type DataMessage struct {
	Type      uint16
	Fields    map[byte][]byte
	DevFields map[byte][]byte
	Error     error
	Arch      byte
}

type FIT struct {
	input       io.Reader
	MessageChan chan DataMessage
}

type DefinitionMesg struct {
	MesgNum     uint16
	Arch        byte
	DevDataFlag byte
	Fields      []FieldDefinition
	DevFields   []FieldDefinition
}

type FieldDefinition struct {
	Number     byte
	Size       byte
	Type       byte
	Endian     bool
	DevDataIdx byte
}

func GetEpoch() time.Time {
	return time.Date(1989, time.December, 31, 0, 0, 0, 0, time.UTC)
}

func NewFIT(input io.Reader) *FIT {
	fit := FIT{input: input}
	fit.MessageChan = make(chan DataMessage)

	return &fit
}

func (f *FIT) parseFieldDefinitions(defMesg *DefinitionMesg, fieldDefs []byte) error {
	defMesg.Fields = make([]FieldDefinition, 0)

	for i := 0; i < len(fieldDefs); i++ {
		fd := FieldDefinition{}
		fd.Number = fieldDefs[i]
		i++

		if i >= len(fieldDefs) {
			return errors.New("invalid fit file: field definition format incorrect")
		}

		fd.Size = fieldDefs[i]
		i++

		if i >= len(fieldDefs) {
			return errors.New("invalid fit file: field definition format incorrect")
		}

		if (fieldDefs[i] & 64) == 64 {
			fd.Endian = true
		}
		fd.Type = fieldDefs[i] & 15

		defMesg.Fields = append(defMesg.Fields, fd)
	}

	return nil
}

func (f *FIT) parseDevFieldDefinitions(defMesg *DefinitionMesg, fieldDefs []byte) error {
	defMesg.DevFields = make([]FieldDefinition, 0)

	for i := 0; i < len(fieldDefs); i++ {
		fd := FieldDefinition{}
		fd.Number = fieldDefs[i]
		i++

		if i >= len(fieldDefs) {
			return errors.New("invalid fit file: dev field definition format incorrect")
		}

		fd.Size = fieldDefs[i]
		i++

		if i >= len(fieldDefs) {
			return errors.New("invalid fit file: dev field definition format incorrect")
		}

		fd.DevDataIdx = fieldDefs[i]

		defMesg.DevFields = append(defMesg.DevFields, fd)
	}

	return nil
}

func (f *FIT) parseDataMessage(defMesg *DefinitionMesg) (DataMessage, error) {
	dataMsg := DataMessage{}
	dataMsg.Type = defMesg.MesgNum
	dataMsg.Fields = make(map[byte][]byte)
	dataMsg.DevFields = make(map[byte][]byte)
	dataMsg.Arch = defMesg.Arch

	for _, field := range defMesg.Fields {
		dataMsg.Fields[field.Number] = make([]byte, field.Size)
		br, derr := f.input.Read(dataMsg.Fields[field.Number])
		if derr != nil || br <= 0 {
			return dataMsg, derr
		}
	}

	for _, field := range defMesg.DevFields {
		dataMsg.DevFields[field.Number] = make([]byte, field.Size)
		br, derr := f.input.Read(dataMsg.DevFields[field.Number])
		if derr != nil || br <= 0 {
			return dataMsg, derr
		}
	}

	return dataMsg, nil
}

func (f *FIT) Parse() {
	go f.parse()
}

func (f *FIT) parse() {
	// Parse the header
	headerLen := make([]byte, 1)
	br, re := f.input.Read(headerLen)
	if re != nil || br <= 0 {
		f.MessageChan <- DataMessage{Error: re}
		close(f.MessageChan)
		return
	}

	// Seek ahead past the header now that we know its length
	header := make([]byte, headerLen[0]-1)
	br, re = f.input.Read(header)
	if re != nil || br <= 0 {
		f.MessageChan <- DataMessage{Error: re}
		close(f.MessageChan)
		return
	}

	// Declare what you can up front to avoid unnecessary gc
	recordHeader := make([]byte, 1)
	reserved := make([]byte, 1)
	arch := make([]byte, 1)
	globalMsgNum := make([]byte, 2)
	numFields := make([]byte, 1)
	numDevFields := make([]byte, 1)

	localMessageTypes := make(map[byte]DefinitionMesg)

	for true {
		// Read the record header
		br, re = f.input.Read(recordHeader)
		if re != nil || br <= 0 {
			f.MessageChan <- DataMessage{Error: re}
			close(f.MessageChan)
			return
		}

		// If this is a definition message
		if (recordHeader[0] & 64) == 64 {
			currentDefinition := DefinitionMesg{}
			currentDefinition.DevDataFlag = recordHeader[0] & 32

			localMessageType := recordHeader[0] & 15

			// Read the reserved
			br, re = f.input.Read(reserved)
			if re != nil || br <= 0 {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			br, re = f.input.Read(arch)
			if re != nil || br <= 0 {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}
			currentDefinition.Arch = arch[0]

			// Read the global message number
			br, re = f.input.Read(globalMsgNum)
			if re != nil || br <= 0 {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			// Check the arch field to determine the endianness of the global mesg num
			if int64(arch[0]) == 0 {
				currentDefinition.MesgNum = binary.LittleEndian.Uint16(globalMsgNum)
			} else {
				currentDefinition.MesgNum = binary.BigEndian.Uint16(globalMsgNum)
			}

			// Read the number of fields
			br, re = f.input.Read(numFields)
			if re != nil || br <= 0 {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			// Read the full block of field definitions and then parse them
			fieldDefinitions := make([]byte, 3*int(numFields[0]))
			br, re := f.input.Read(fieldDefinitions)
			if re != nil || br <= 0 {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}
			pfd := f.parseFieldDefinitions(&currentDefinition, fieldDefinitions)
			if pfd != nil {
				f.MessageChan <- DataMessage{Error: pfd}
				close(f.MessageChan)
				return
			}

			// If the developer data flag is set, read the dev data fields
			if currentDefinition.DevDataFlag > 0 {
				// Read the number of fields
				br, re = f.input.Read(numDevFields)
				if re != nil || br <= 0 {
					f.MessageChan <- DataMessage{Error: re}
					close(f.MessageChan)
					return
				}

				devFieldDefinitions := make([]byte, 3*numDevFields[0])
				br, re := f.input.Read(devFieldDefinitions)
				if re != nil || br <= 0 {
					f.MessageChan <- DataMessage{Error: re}
					close(f.MessageChan)
					return
				}

				pfd := f.parseDevFieldDefinitions(&currentDefinition, devFieldDefinitions)
				if pfd != nil {
					f.MessageChan <- DataMessage{Error: pfd}
					close(f.MessageChan)
					return
				}
			}

			// Add this local message type to map
			localMessageTypes[localMessageType] = currentDefinition
		} else {
			// Parse the local message type of this data message then look for its definition in the map
			localMessageType := recordHeader[0] & 15
			currentDefinition := localMessageTypes[localMessageType]

			// Now parse the data msg
			dataMsg, dataErr := f.parseDataMessage(&currentDefinition)
			if dataErr != nil {
				f.MessageChan <- DataMessage{Error: dataErr}
				close(f.MessageChan)
				return
			}

			// And send the result to the result channel
			f.MessageChan <- dataMsg
		}

	}

}
