#!/usr/bin/env python3
"""Derives the platform icons from the ArkPeak application icon.

The application icon is drawn by make-icon.py. This turns it into the
formats each platform needs:

- Windows wants a .ico with several sizes in it.
- macOS wants an .icns, and a separate icon for the .app inside a .dmg.
- Linux wants a PNG at each of the usual icon theme sizes.

The .icns is a container of PNGs. Pillow can write one, but the result is
missing the type markers macOS expects for some sizes, so the container is
assembled here.

Usage:
    python3 build/icon/make-platform-icons.py
"""

import pathlib
import struct

from PIL import Image

ICON_DIR = pathlib.Path(__file__).resolve().parent
BUILD_DIR = ICON_DIR.parent

# The source icon.
SOURCE = ICON_DIR / "appicon.png"

# The sizes Windows puts in the .ico.
WINDOWS_SIZES = [16, 24, 32, 48, 64, 128, 256]

# The sizes macOS puts in the .icns, as (type, pixel size) pairs. The type
# is the four-character code that identifies the entry.
ICNS_TYPES = [
    (b"icp4", 16),
    (b"icp5", 32),
    (b"icp6", 64),
    (b"ic07", 128),
    (b"ic08", 256),
    (b"ic09", 512),
    (b"ic10", 1024),
]

# The sizes the Linux icon theme uses.
LINUX_SIZES = [16, 32, 48, 64, 128, 256, 512]


def write_icns(path, image):
    """Writes an .icns file holding the given sizes.

    The format is a header followed by the entries. Each entry has a
    four-character type, its own length, and the PNG data.
    """
    entries = []
    for kind, size in ICNS_TYPES:
        resized = image.resize((size, size), Image.LANCZOS)
        data = png_bytes(resized)
        entries.append((kind, data))

    body = b"".join(
        kind + struct.pack(">I", len(data) + 8) + data for kind, data in entries
    )
    path.write_bytes(b"icns" + struct.pack(">I", len(body) + 8) + body)


def png_bytes(image):
    """Encodes an image as PNG and returns the bytes."""
    import io

    buffer = io.BytesIO()
    image.save(buffer, format="PNG")
    return buffer.getvalue()


def main():
    image = Image.open(SOURCE).convert("RGBA")

    # Windows.
    ico = BUILD_DIR / "windows" / "icon.ico"
    image.save(ico, format="ICO", sizes=[(s, s) for s in WINDOWS_SIZES])
    print("wrote", ico)

    # macOS: the application icon and the one shown inside a disk image.
    write_icns(BUILD_DIR / "darwin" / "icons.icns", image)
    print("wrote", BUILD_DIR / "darwin" / "icons.icns")

    image.save(BUILD_DIR / "darwin" / "dmg-file-icon.png", format="PNG")
    write_icns(BUILD_DIR / "darwin" / "dmg-file-icon.icns", image)
    print("wrote", BUILD_DIR / "darwin" / "dmg-file-icon.icns")

    # Linux: one PNG per icon theme size.
    linux = BUILD_DIR / "linux" / "icons"
    linux.mkdir(parents=True, exist_ok=True)
    for size in LINUX_SIZES:
        resized = image.resize((size, size), Image.LANCZOS)
        resized.save(linux / f"{size}x{size}.png", format="PNG")
    print("wrote", linux)


if __name__ == "__main__":
    main()
