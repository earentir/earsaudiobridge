package audio

import "encoding/binary"

// ApplyGain scales s16le PCM samples in place.
func ApplyGain(pcm []byte, gain float64) {
	if gain == 1 || len(pcm) < 2 {
		return
	}
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i : i+2]))) * gain
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(pcm[i:i+2], uint16(int16(v)))
	}
}
