gofit
=========

The gofit package provides a basic library for parsing FIT (Flexible and Interoperable Data Transfer) files.

#### To install
	
	go get github.com/kcfwpi/gofit

#### To use

	import "github.com/kcfwpi/gofit"

<a name="examples"></a>Examples
-------------------------------

Use the NewFIT function to create a new FIT. The only parameter is a type that implements the io.Reader interface. This is often a file type.

		fit := NewFIT(f)
		
Call the Parse function on the returned FIT struct. This will start a goroutine that begins to stream through the io.Reader that contains the FIT data.

		fit.Parse()

FIT data messages are sent to the FIT struct message channel as they are parsed. Range over this channel to begin reading the messages. When the end of reader is reached the MessageChan is closed.

	for m := range fit.MessageChan {
		// work on Message here
	}

