package codec

import "errors"

// ErrBuffering means the encoder is accumulating input and has no wire output yet.
var ErrBuffering = errors.New("codec: frame not ready")
