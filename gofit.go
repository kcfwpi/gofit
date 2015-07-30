package gofit

import (
	"encoding/binary"
	"io"
	"os"
)

type DataMessage struct {
	Type   uint16
	Fields map[byte][]byte
	Error  error
}

type FIT struct {
	file        string
	MessageChan chan DataMessage
}

type DefinitionMesg struct {
	MesgNum uint16
	Fields  []FieldDefinition
}

type FieldDefinition struct {
	Number byte
	Size   byte
	Type   byte
	Endian bool
}

func NewFIT(file string) *FIT {
	fit := FIT{file: file}
	fit.MessageChan = make(chan DataMessage)

	return &fit
}

func parseFieldDefinitions(defMesg *DefinitionMesg, fieldDefs []byte) {
	defMesg.Fields = make([]FieldDefinition, 0)

	for i := 0; i < len(fieldDefs); i++ {
		fd := FieldDefinition{}
		fd.Number = fieldDefs[i]
		i++
		fd.Size = fieldDefs[i]
		i++

		if (fieldDefs[i] & 64) == 64 {
			fd.Endian = true
		}
		fd.Type = fieldDefs[i] & 15

		defMesg.Fields = append(defMesg.Fields, fd)
	}
}

func parseDataMessage(defMesg *DefinitionMesg, data io.Reader) (DataMessage, error) {
	dataMsg := DataMessage{}
	dataMsg.Type = defMesg.MesgNum
	dataMsg.Fields = make(map[byte][]byte)

	for _, field := range defMesg.Fields {
		dataMsg.Fields[field.Number] = make([]byte, field.Size)
		_, derr := data.Read(dataMsg.Fields[field.Number])
		if derr != nil {
			return dataMsg, derr
		}
	}

	return dataMsg, nil
}

func (f *FIT) Parse() {
	go f.parse()
}

func (f *FIT) parse() {
	file, ferr := os.Open(f.file)
	if ferr != nil {
		f.MessageChan <- DataMessage{Error: ferr}
		close(f.MessageChan)
		return
	}
	defer file.Close()

	// Parse the header
	headerLen := make([]byte, 1)
	_, re := file.Read(headerLen)
	if re != nil {
		f.MessageChan <- DataMessage{Error: re}
		close(f.MessageChan)
		return
	}
	headerLenInt, _ := binary.Uvarint(headerLen)

	// Seek ahead past the header now that we know its length
	_, re = file.Seek(int64(headerLenInt), 0)
	if re != nil {
		f.MessageChan <- DataMessage{Error: re}
		close(f.MessageChan)
		return
	}

	// Declare what you can up front to avoid unnecessary gc
	recordHeader := make([]byte, 1)
	arch := make([]byte, 1)
	globalMsgNum := make([]byte, 2)
	numFields := make([]byte, 1)

	localMessageTypes := make(map[byte]DefinitionMesg)

	for true {
		// Read the record header
		_, re = file.Read(recordHeader)
		if re != nil {
			f.MessageChan <- DataMessage{Error: re}
			close(f.MessageChan)
			return
		}

		// If this is a definition message
		if (recordHeader[0] & 64) == 64 {
			currentDefinition := DefinitionMesg{}

			localMessageType := recordHeader[0] & 15

			// Read the reserved
			file.Seek(1, 1)
			_, re = file.Read(arch)
			if re != nil {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			// Read the global message number
			_, re = file.Read(globalMsgNum)
			if re != nil {
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
			_, re = file.Read(numFields)
			if re != nil {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			// Read the full block of field definitions and then parse them
			fieldDefinitions := make([]byte, 3*numFields[0])
			_, re := file.Read(fieldDefinitions)
			if re != nil {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}
			parseFieldDefinitions(&currentDefinition, fieldDefinitions)

			// Add this local message type to map
			localMessageTypes[localMessageType] = currentDefinition
		} else {
			// Parse the local message type of this data message then look for its definition in the map
			localMessageType := recordHeader[0] & 15
			currentDefinition := localMessageTypes[localMessageType]

			// Now parse the data msg
			dataMsg, dataErr := parseDataMessage(&currentDefinition, file)
			if dataErr != nil {
				f.MessageChan <- DataMessage{Error: re}
				close(f.MessageChan)
				return
			}

			// And send the result to the result channel
			f.MessageChan <- dataMsg
		}

	}

}
