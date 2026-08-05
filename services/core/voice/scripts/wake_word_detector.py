#!/usr/bin/env python3
"""wake_word_detector.py: openWakeWord subprocess backing
services/core/voice's WakeWordDetectorImpl (SPEC-0055).

Reads 4-byte-little-endian-length-prefixed PCM frames from stdin (matching
the Go side's framing.go), runs them through an openWakeWord model, and
writes a single "DETECTED" line to stdout whenever the model's score crosses
threshold. wake_word.go's readDetectionLoop treats stdout as plain
newline-delimited text (never binary), so no framing is used in this
direction.

Usage:
    wake_word_detector.py <model_path> [--threshold=0.5]
"""
import argparse
import struct
import sys
import time
from typing import Optional

try:
    import numpy as np
except ImportError:
    print("wake_word_detector.py: numpy is required", file=sys.stderr)
    sys.exit(1)

try:
    from openwakeword.model import Model
except ImportError:
    print("wake_word_detector.py: openwakeword is required (pip install openwakeword)", file=sys.stderr)
    sys.exit(1)


def read_frame() -> Optional[bytes]:
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


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("model_path")
    parser.add_argument("--threshold", type=float, default=0.5)
    parser.add_argument("--cooldown", type=float, default=2.0)
    args = parser.parse_args()

    model = Model(wakeword_models=[args.model_path])
    last_detection = 0.0

    while True:
        frame = read_frame()
        if frame is None:
            break
        if not frame:
            continue
        samples = np.frombuffer(frame, dtype=np.int16)
        predictions = model.predict(samples)
        now = time.monotonic()
        if any(score >= args.threshold for score in predictions.values()):
            if now - last_detection >= args.cooldown:
                print("DETECTED", flush=True)
                last_detection = now


if __name__ == "__main__":
    main()
