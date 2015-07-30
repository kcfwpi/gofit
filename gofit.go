package gofit

import (
	"encoding/binary"
	"fmt"
	"os"
)

type Message struct {
	Type   string
	Fields map[string]string
}

func createErrorMessage(e error) Message {
	m := Message{}
	m.Type = "error"

	m.Fields = make(map[string]string)
	m.Fields["message"] = e.Error()

	return m
}

type FIT struct {
	file        string
	MessageChan chan Message
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
	fit.MessageChan = make(chan Message)

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

func (f *FIT) Parse() {
	file, ferr := os.Open(f.file)
	if ferr != nil {
		f.MessageChan <- createErrorMessage(ferr)
		close(f.MessageChan)
		return
	}
	defer file.Close()

	// Parse the header
	headerLen := make([]byte, 1)
	_, re := file.Read(headerLen)
	if re != nil {
		f.MessageChan <- createErrorMessage(re)
		close(f.MessageChan)
		return
	}
	headerLenInt, _ := binary.Uvarint(headerLen)

	fmt.Printf("headerlenint: %d\n", headerLenInt)

	// Seek ahead past the header now that we know its length
	_, re = file.Seek(int64(headerLenInt), 0)
	if re != nil {
		f.MessageChan <- createErrorMessage(re)
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
			f.MessageChan <- createErrorMessage(re)
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
				f.MessageChan <- createErrorMessage(re)
				close(f.MessageChan)
				return
			}

			// Read the global message number
			_, re = file.Read(globalMsgNum)
			if re != nil {
				f.MessageChan <- createErrorMessage(re)
				close(f.MessageChan)
				return
			}

			if int64(arch[0]) == 0 {
				currentDefinition.MesgNum = binary.LittleEndian.Uint16(globalMsgNum)
			} else {
				currentDefinition.MesgNum = binary.BigEndian.Uint16(globalMsgNum)
			}

			// Read the number of fields
			_, re = file.Read(numFields)
			if re != nil {
				f.MessageChan <- createErrorMessage(re)
				close(f.MessageChan)
				return
			}

			fieldDefinitions := make([]byte, 3*numFields[0])
			_, re := file.Read(fieldDefinitions)
			if re != nil {
				f.MessageChan <- createErrorMessage(re)
				close(f.MessageChan)
				return
			}

			parseFieldDefinitions(&currentDefinition, fieldDefinitions)

			localMessageTypes[localMessageType] = currentDefinition
		} else {
			localMessageType := recordHeader[0] & 15
			currentDefinition := localMessageTypes[localMessageType]

			fmt.Printf("%d\n", currentDefinition.MesgNum)

			for _, field := range currentDefinition.Fields {
				fieldData := make([]byte, field.Size)

				_, re := file.Read(fieldData)
				if re != nil {
					f.MessageChan <- createErrorMessage(re)
					close(f.MessageChan)
					return
				}

			}

		}

	}

}
