#!/usr/bin/env python3
"""whisper_stt.py: faster-whisper subprocess backing services/core/voice's
WhisperProvider (SPEC-0057), the first concrete core.STTProvider (SPEC-0056).

Two subcommands, matching WhisperProvider's two STTProvider methods:

    whisper_stt.py transcribe --model=base.en --language=en --device=cpu
        Reads one complete raw PCM buffer (mono, int16 LE, matching
        VoiceEngine.Capture's chunk format) from stdin until EOF - the same
        single-buffer-per-invocation convention audio_engine.py's "playback"
        mode uses - transcribes it whole, and writes exactly one JSON line
        to stdout: {"text": ..., "confidence": ...}.

    whisper_stt.py stream --model=base.en --language=en --device=cpu \
        --sample-rate=16000 --segment-seconds=3
        Reads 4-byte-little-endian-length-prefixed PCM frames from stdin
        (matching the Go side's framing.go, the same protocol
        wake_word_detector.py already uses for its input), accumulating them
        into segment-seconds-long segments. Each time a segment fills (or
        stdin closes with a non-empty partial segment left over), that
        segment is transcribed independently and one JSON line is written to
        stdout: {"text": ..., "confidence": ..., "done": true}.

        Whisper has no native partial/incremental decoding mode - unlike
        wake word detection, a transcription can't be meaningfully updated
        word-by-word mid-utterance. "done" is therefore always true: each
        line is a complete transcription of one fixed-length segment, not a
        provisional guess later revised. This is the standard segment-based
        approach local voice assistants use to get streaming-shaped output
        from a non-streaming model, in the same spirit as VAD-chunked
        Whisper pipelines.

Confidence is derived from faster-whisper's per-segment avg_logprob (a log
probability, so always <= 0): confidence = exp(avg_logprob), which maps a
segment's confidence onto Whisper's own [0, 1] convention rather than
inventing a new one.
"""
import argparse
import json
import math
import struct
import sys
from typing import Optional, Tuple

try:
    import numpy as np
except ImportError:
    print("whisper_stt.py: numpy is required", file=sys.stderr)
    sys.exit(1)

try:
    from faster_whisper import WhisperModel
except ImportError:
    print("whisper_stt.py: faster-whisper is required (pip install faster-whisper)", file=sys.stderr)
    sys.exit(1)

# compute_type per device: int8 is the fast, low-memory choice for CPU
# inference; float16 is faster-whisper's standard choice for CUDA.
COMPUTE_TYPE_BY_DEVICE = {"cpu": "int8", "cuda": "float16"}


def pcm_to_float32(pcm: bytes) -> np.ndarray:
    """Converts raw int16 LE PCM bytes to the normalized float32 [-1, 1]
    array faster-whisper's transcribe() expects."""
    samples = np.frombuffer(pcm, dtype="<i2")
    return samples.astype(np.float32) / 32768.0


def load_model(model_size: str, device: str) -> WhisperModel:
    compute_type = COMPUTE_TYPE_BY_DEVICE.get(device, "default")
    return WhisperModel(model_size, device=device, compute_type=compute_type)


def transcribe_segment(model: WhisperModel, pcm: bytes, language: str) -> Tuple[str, float]:
    """Transcribes one PCM buffer, returning (text, confidence). confidence
    is the mean of exp(avg_logprob) across segments faster-whisper found (0
    if it found none, i.e. silence)."""
    audio = pcm_to_float32(pcm)
    segments, _info = model.transcribe(audio, language=language or None)

    texts = []
    logprobs = []
    for segment in segments:
        texts.append(segment.text.strip())
        logprobs.append(segment.avg_logprob)

    text = " ".join(t for t in texts if t)
    confidence = math.exp(sum(logprobs) / len(logprobs)) if logprobs else 0.0
    return text, confidence


def write_result(text: str, confidence: float, done: Optional[bool] = None) -> None:
    result = {"text": text, "confidence": confidence}
    if done is not None:
        result["done"] = done
    print(json.dumps(result), flush=True)


def cmd_transcribe(args: argparse.Namespace) -> None:
    model = load_model(args.model, args.device)
    pcm = sys.stdin.buffer.read()
    if not pcm:
        write_result("", 0.0)
        return
    text, confidence = transcribe_segment(model, pcm, args.language)
    write_result(text, confidence)


def read_frame() -> Optional[bytes]:
    """Reads one length-prefixed frame from stdin (framing.go's protocol).
    Returns None at a genuine end of stream (stdin closed before or during a
    frame) - distinct from b"", a valid zero-length frame that must be
    skipped rather than treated as stream end, matching
    audio_engine.go/readCaptureLoop's own "if len(frame) == 0 { continue }"
    handling of the same protocol."""
    header = sys.stdin.buffer.read(4)
    if len(header) < 4:
        return None
    (length,) = struct.unpack("<I", header)
    if length == 0:
        return b""
    payload = sys.stdin.buffer.read(length)
    if len(payload) < length:
        return None
    return payload


def cmd_stream(args: argparse.Namespace) -> None:
    model = load_model(args.model, args.device)
    segment_bytes = int(args.sample_rate * args.segment_seconds) * 2  # int16 = 2 bytes/sample

    buf = bytearray()
    while True:
        frame = read_frame()
        if frame is None:
            break
        if not frame:
            continue
        buf.extend(frame)
        if len(buf) >= segment_bytes:
            text, confidence = transcribe_segment(model, bytes(buf), args.language)
            if text:
                write_result(text, confidence, done=True)
            buf.clear()

    if buf:
        text, confidence = transcribe_segment(model, bytes(buf), args.language)
        if text:
            write_result(text, confidence, done=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="mode", required=True)

    transcribe_parser = subparsers.add_parser("transcribe")
    transcribe_parser.add_argument("--model", default="base.en")
    transcribe_parser.add_argument("--language", default="")
    transcribe_parser.add_argument("--device", default="cpu")

    stream_parser = subparsers.add_parser("stream")
    stream_parser.add_argument("--model", default="base.en")
    stream_parser.add_argument("--language", default="")
    stream_parser.add_argument("--device", default="cpu")
    stream_parser.add_argument("--sample-rate", type=int, default=16000)
    stream_parser.add_argument("--segment-seconds", type=float, default=3.0)

    args = parser.parse_args()

    if args.mode == "transcribe":
        cmd_transcribe(args)
    else:
        cmd_stream(args)


if __name__ == "__main__":
    main()
