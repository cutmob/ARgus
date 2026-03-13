/**
 * ARGUS Audio Worklet Processor
 *
 * Runs in the AudioWorklet thread (separate from the main thread) for
 * lowest-latency mic capture. Emits ~20ms PCM16 chunks at 16kHz to the
 * main thread, aligned with Google Gemini Live API best-practice chunk sizing
 * (20–40ms recommended in the Live API documentation, 2025).
 *
 * VAD energy gate: chunks below the RMS threshold are not forwarded,
 * preventing background noise from consuming Gemini token budget.
 */

const TARGET_SAMPLE_RATE = 16000;
// 320 samples at 16kHz = exactly 20ms per chunk
const OUTPUT_CHUNK_SAMPLES = 320;
// RMS energy gate — chunks quieter than this are treated as silence.
// 0.005 ≈ -46 dBFS, well below conversational speech.
const VAD_RMS_THRESHOLD = 0.005;

class ArgusAudioProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    // Accumulation buffer: we accumulate resampled 16kHz samples here until
    // we have OUTPUT_CHUNK_SAMPLES to emit.
    this._buffer = new Float32Array(OUTPUT_CHUNK_SAMPLES * 4);
    this._bufferLen = 0;
    // Ratio from browser sample rate → 16kHz is computed on first process().
    this._ratio = null;
    // Fractional offset accumulator for the downsampling loop.
    this._offsetFrac = 0;
  }

  process(inputs) {
    const input = inputs[0];
    if (!input || !input[0] || input[0].length === 0) return true;

    const raw = input[0]; // Float32, browser sample rate, mono

    // Compute downsampling ratio on first frame (sampleRate is a global in worklet scope)
    if (this._ratio === null) {
      this._ratio = sampleRate / TARGET_SAMPLE_RATE;
    }

    // Compute RMS energy of this input frame for VAD gating
    let sumSq = 0;
    for (let i = 0; i < raw.length; i++) sumSq += raw[i] * raw[i];
    const rms = Math.sqrt(sumSq / raw.length);

    if (rms < VAD_RMS_THRESHOLD) {
      // Below energy threshold — treat as silence, do not forward
      return true;
    }

    // Downsample to TARGET_SAMPLE_RATE using averaging (same method as
    // the previous ScriptProcessor path, preserved for quality consistency).
    const ratio = this._ratio;
    const outputLen = Math.round(raw.length / ratio);

    for (let out = 0; out < outputLen; out++) {
      const start = Math.round((out + this._offsetFrac) * ratio);
      const end = Math.min(raw.length, Math.round((out + 1 + this._offsetFrac) * ratio));
      let acc = 0;
      let count = 0;
      for (let i = start; i < end; i++) {
        acc += raw[i];
        count++;
      }
      const sample = Math.max(-1, Math.min(1, count > 0 ? acc / count : 0));

      // Grow buffer if needed (rare: only on first call with large ratio)
      if (this._bufferLen >= this._buffer.length) {
        const bigger = new Float32Array(this._buffer.length * 2);
        bigger.set(this._buffer);
        this._buffer = bigger;
      }
      this._buffer[this._bufferLen++] = sample;

      // Emit chunk when we have OUTPUT_CHUNK_SAMPLES accumulated
      if (this._bufferLen >= OUTPUT_CHUNK_SAMPLES) {
        const pcm16 = floatToPCM16(this._buffer.subarray(0, OUTPUT_CHUNK_SAMPLES));
        this.port.postMessage({ pcm16 }, [pcm16.buffer]);
        // Shift remaining samples to front
        const remaining = this._bufferLen - OUTPUT_CHUNK_SAMPLES;
        this._buffer.copyWithin(0, OUTPUT_CHUNK_SAMPLES, this._bufferLen);
        this._bufferLen = remaining;
      }
    }

    return true; // Keep processor alive
  }
}

/**
 * Convert a Float32Array of normalised [-1, 1] samples to PCM16 Int16Array.
 */
function floatToPCM16(float32) {
  const pcm = new Int16Array(float32.length);
  for (let i = 0; i < float32.length; i++) {
    const s = Math.max(-1, Math.min(1, float32[i]));
    pcm[i] = s < 0 ? s * 32768 : s * 32767;
  }
  return pcm;
}

registerProcessor("argus-audio-processor", ArgusAudioProcessor);
