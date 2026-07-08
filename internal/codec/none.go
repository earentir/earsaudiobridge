package codec

// None passes PCM through unchanged.
type None struct{}

// NewNone returns an uncompressed codec.
func NewNone() *None { return &None{} }

// Info implements Codec.
func (c *None) Info() Info { return Info{Name: "none"} }

// Encode implements Codec.
func (c *None) Encode(pcm []byte) ([]byte, error) {
	out := make([]byte, len(pcm))
	copy(out, pcm)
	return out, nil
}

// Decode implements Codec.
func (c *None) Decode(wire []byte) ([]byte, error) {
	out := make([]byte, len(wire))
	copy(out, wire)
	return out, nil
}
