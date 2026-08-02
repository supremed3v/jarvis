#!/usr/bin/env python3
"""audio_engine.py: capture/playback subprocess backing services/core/voice's
AudioEngine (SPEC-0053).

Speaks the same 4-byte-little-endian-length-prefixed framing as the Go
side's framing.go for the "capture" subcommand's stdout stream. "playback"
reads raw PCM from stdin until EOF (a single buffer per invocation, so no
framing is needed there).

Usage:
    audio_engine.py capture --sample-rate=16000 --device=default
    audio_engine.py playback --sample-rate=16000
"""
import argparse
import struct
import sys

try:
    import sounddevice as sd
except ImportError:
    print("audio_engine.py: sounddevice is required (pip install sounddevice)", file=sys.stderr)
    sys.exit(1)

FRAME_MS = 20  # 20ms blocks - a common low-latency choice for real-time audio


def write_frame(payload: bytes) -> None:
    sys.stdout.buffer.write(struct.pack("<I", len(payload)))
    sys.stdout.buffer.write(payload)
    sys.stdout.buffer.flush()


def resolve_device(name: str):
    if not name or name == "default":
        return None
    return name


def capture(sample_rate: int, device: str) -> None:
    block_size = int(sample_rate * FRAME_MS / 1000)
    dev = resolve_device(device)

    def on_block(indata, frames, time_info, status):
        if status:
            print(f"audio_engine.py: capture status: {status}", file=sys.stderr)
        write_frame(bytes(indata))

    with sd.RawInputStream(
        samplerate=sample_rate,
        channels=1,
        dtype="int16",
        blocksize=block_size,
        device=dev,
        callback=on_block,
    ):
        try:
            while True:
                sd.sleep(1000)
        except KeyboardInterrupt:
            pass


def playback(sample_rate: int) -> None:
    data = sys.stdin.buffer.read()
    if not data:
        return
    with sd.RawOutputStream(samplerate=sample_rate, channels=1, dtype="int16") as stream:
        stream.write(data)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["capture", "playback"])
    parser.add_argument("--sample-rate", type=int, default=16000)
    parser.add_argument("--device", type=str, default="default")
    args = parser.parse_args()

    if args.mode == "capture":
        capture(args.sample_rate, args.device)
    else:
        playback(args.sample_rate)


if __name__ == "__main__":
    main()
