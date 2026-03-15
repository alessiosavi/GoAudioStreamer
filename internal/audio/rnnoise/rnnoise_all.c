// rnnoise_all.c — CGo compilation bridge.
// Includes all vendored RNNoise C sources so they are compiled by CGo.
#include "rnnoise_src/src/denoise.c"
#include "rnnoise_src/src/rnn.c"
#include "rnnoise_src/src/rnnoise_data.c"
#include "rnnoise_src/src/rnnoise_tables.c"
#include "rnnoise_src/src/pitch.c"
#include "rnnoise_src/src/kiss_fft.c"
#include "rnnoise_src/src/celt_lpc.c"
#include "rnnoise_src/src/nnet.c"
#include "rnnoise_src/src/nnet_default.c"
#include "rnnoise_src/src/parse_lpcnet_weights.c"
