package core

// #cgo  CFLAGS:-mpopcnt

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"errors"

	log "github.com/cihub/seelog"
	"github.com/umbel/pilosa/index"
)

func copy_raw(src [32]uint64) index.BlockArray {
	var o = make([]uint64, 32, 32)
	for k, v := range src {
		o[k] = v
	}
	return index.BlockArray{Block: o}
}
func sendBitmap(batcher *Batcher, bitmap index.IBitmap, db string, frame string, bitmap_id, filter uint64, slice int, finish chan error) {
	if slice < 0 {
		log.Warn("Bad split", db, frame, slice, bitmap_id)
		finish <- errors.New("BadSplit")
		return
	}
	compressed_bitmap := bitmap.ToCompressString()
	results := batcher.Batch(db, frame, compressed_bitmap, bitmap_id, slice, filter)
	finish <- results
}

func FromApiString(batcher *Batcher, db string, frame string, api_string string, bitmap_id, filter uint64) string {
	compressed_data, err := base64.StdEncoding.DecodeString(api_string)
	if err != nil {
		log.Warn(err)
		return "Bad"
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed_data))
	if err != nil {
		log.Warn(err)
		return "Bad"
	}
	var numChunks uint64
	err = binary.Read(reader, binary.LittleEndian, &numChunks)
	if err != nil {
		log.Warn(err)
		return "Bad"
	}
	first := true
	bitmap := index.NewBitmap()
	last_slice := index.COUNTERMASK
	sent_count := 0
	finish := make(chan error)

	for i := uint64(0); i < numChunks; i++ {
		var raw struct {
			Key   uint64
			Block [32]uint64
		}
		binary.Read(reader, binary.LittleEndian, &raw)
		slice := raw.Key >> 5
		if slice != last_slice {
			if first {
				first = false
			} else {
				//make async later
				sent_count += 1
				go sendBitmap(batcher, bitmap, db, frame, bitmap_id, filter, int(last_slice), finish)
				bitmap = index.NewBitmap()
			}
			last_slice = slice
		}
		o := copy_raw(raw.Block)
		chunk := &index.Chunk{Key: raw.Key, Value: o}
		bitmap.AddChunk(chunk)

	}
	sent_count += 1
	go sendBitmap(batcher, bitmap, db, frame, bitmap_id, filter, int(last_slice), finish)
	for i := 0; i < sent_count; i++ {
		<-finish
	}
	return "OK"

}
