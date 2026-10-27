#!/usr/bin/env python3
"""Draws the ArkPeak application icon.

The icon is a hex cell with a peak rising out of it: the hex is the grid
the town is built on, and the peak is the town itself. The colours come
from the game's palette, so the icon and the game look like the same
thing.

The script writes the PNG the build uses and an SVG that can be edited in
a vector editor later.

Usage:
    python3 build/icon/make-icon.py
"""

import math
import pathlib

from PIL import Image, ImageDraw, ImageFilter

# The size of the PNG the build uses. The icon is drawn at four times this
# and downsampled, so the edges stay smooth.
SIZE = 1024
SUPERSAMPLE = 4

# The game's palette, from frontend/src/render/palette.ts.
SKY = (207, 220, 224)
WATER = (143, 176, 189)
CREAM = (242, 236, 224)
ROSE = (228, 191, 180)
SAGE = (194, 205, 180)

# The accent used for the peak's shadow side.
DEEP_ROSE = (188, 141, 132)

# The outline colour of the hex cell.
OUTLINE = (150, 142, 128)

OUT_DIR = pathlib.Path(__file__).resolve().parent


def hex_points(cx, cy, radius, rotation=0.0):
    """The corners of a flat-topped hexagon.

    The game draws its cells as flat-topped hexagons: the cell's flat
    sides face the six neighbours, and its corners point east and west.
    The icon uses the same shape, so the app icon and the grid look like
    the same thing.
    """
    points = []
    for i in range(6):
        angle = rotation + i * math.pi / 3
        points.append(
            (cx + radius * math.cos(angle), cy + radius * math.sin(angle))
        )
    return points


def draw_icon(size=SIZE):
    """Draws the icon at the given size and returns the image."""
    scale = SUPERSAMPLE
    canvas = size * scale

    image = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)

    def s(value):
        """Scales a length to the supersampled canvas."""
        return value * scale * size / SIZE

    # The background: a soft sky that fades towards the water at the
    # bottom, so the icon has a ground to stand on.
    for y in range(canvas):
        t = y / canvas
        if t < 0.7:
            colour = SKY
        else:
            u = (t - 0.7) / 0.3
            colour = tuple(
                int(SKY[i] + (WATER[i] - SKY[i]) * u) for i in range(3)
            )
        draw.line([(0, y), (canvas, y)], fill=colour + (255,))

    # A soft sun in the upper left. It is drawn as a radial fade on an
    # opaque layer, so the blur cannot leave a dark rim against the
    # transparent background.
    sun = Image.new("RGBA", (canvas, canvas), SKY + (255,))
    sun_draw = ImageDraw.Draw(sun)
    sun_r = s(150)
    sun_cx, sun_cy = s(235), s(215)
    for i in range(40, 0, -1):
        r = sun_r * i / 40
        alpha = int(46 * (1 - i / 40))
        sun_draw.ellipse(
            [sun_cx - r, sun_cy - r, sun_cx + r, sun_cy + r],
            fill=(255, 250, 235, alpha),
        )
    sun = sun.filter(ImageFilter.GaussianBlur(s(22)))
    image = Image.alpha_composite(image, sun)
    draw = ImageDraw.Draw(image)

    # The hex cell, sitting on the water. It is the ground the peak
    # stands on.
    cx = canvas / 2
    cy = s(560)
    radius = s(300)

    # The reflection on the water: the cell mirrored downwards, fading
    # out towards the bottom. It is drawn as a gradient rather than a
    # solid shape, so it reads as a reflection and not as a second cell.
    reflection = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    reflection_draw = ImageDraw.Draw(reflection)
    mirrored = hex_points(cx, cy + s(230), radius)
    reflection_draw.polygon(mirrored, fill=WATER + (255,))

    # Fade the reflection out towards the bottom.
    fade = Image.new("L", (canvas, canvas), 0)
    fade_draw = ImageDraw.Draw(fade)
    top = int(cy + s(90))
    bottom = int(cy + s(330))
    for y in range(top, min(bottom, canvas)):
        t = (y - top) / max(1, bottom - top)
        value = int(210 * (1 - t))
        fade_draw.line([(0, y), (canvas, y)], fill=value)
    reflection.putalpha(
        Image.composite(
            reflection.getchannel("A"), Image.new("L", (canvas, canvas), 0), fade
        )
    )
    reflection = reflection.filter(ImageFilter.GaussianBlur(s(4)))
    image = Image.alpha_composite(image, reflection)
    draw = ImageDraw.Draw(image)

    # The cell itself.
    draw.polygon(hex_points(cx, cy, radius), fill=CREAM + (255,))
    draw.polygon(
        hex_points(cx, cy, radius), outline=OUTLINE + (255,), width=int(s(7))
    )

    # The peak. Its base sits on the cell's lower half and it rises past
    # the cell's top edge, so it reads as standing on the ground rather
    # than floating above it.
    peak_cx = cx
    peak_cy = s(690)
    half = s(150)
    height = s(520)
    summit_x = peak_cx + s(6)
    shoulder_x = peak_cx + s(80)
    shoulder_y = peak_cy - height * 0.56

    # The shadowed face, drawn first so the lit face overlaps it.
    draw.polygon(
        [
            (summit_x, peak_cy - height),
            (shoulder_x, shoulder_y),
            (peak_cx + half, peak_cy),
            (peak_cx + s(6), peak_cy),
        ],
        fill=DEEP_ROSE + (255,),
    )

    # The lit face.
    draw.polygon(
        [
            (peak_cx - half, peak_cy),
            (summit_x, peak_cy - height),
            (shoulder_x, shoulder_y),
            (peak_cx + s(6), peak_cy),
        ],
        fill=ROSE + (255,),
    )

    # Snow on the summit, following the two faces. It covers the top of
    # the peak, on both the lit and the shadowed side.
    draw.polygon(
        [
            (summit_x, peak_cy - height),
            (shoulder_x, shoulder_y),
            (summit_x + s(44), peak_cy - height * 0.78),
            (summit_x - s(32), peak_cy - height * 0.72),
        ],
        fill=CREAM + (255,),
    )

    # A couple of trees on the cell, so the icon shows what the game is
    # about: a place with things growing on it. They sit near the bottom
    # corners, where the cell is widest, so neither is cut off.
    for tx, ty, tr in ((s(300), s(700), s(46)), (s(724), s(700), s(46))):
        draw.polygon(
            [(tx, ty - tr * 1.9), (tx - tr * 0.72, ty), (tx + tr * 0.72, ty)],
            fill=SAGE + (255,),
        )
        draw.polygon(
            [
                (tx, ty - tr * 2.7),
                (tx - tr * 0.6, ty - tr * 0.85),
                (tx + tr * 0.6, ty - tr * 0.85),
            ],
            fill=(174, 190, 156, 255),
        )

    return image.resize((size, size), Image.LANCZOS)


SVG = """<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" width="1024" height="1024">
  <!-- The ArkPeak icon: a hex cell with a peak rising out of it.
       This file is the editable source; make-icon.py renders it to PNG. -->
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#cfdce0"/>
      <stop offset="0.7" stop-color="#cfdce0"/>
      <stop offset="1" stop-color="#8fb0bd"/>
    </linearGradient>
    <radialGradient id="sun">
      <stop offset="0" stop-color="#fffaeb" stop-opacity="0.35"/>
      <stop offset="1" stop-color="#fffaeb" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="reflection" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#8fb0bd" stop-opacity="0.8"/>
      <stop offset="1" stop-color="#8fb0bd" stop-opacity="0"/>
    </linearGradient>
  </defs>

  <rect width="1024" height="1024" fill="url(#sky)"/>
  <circle cx="235" cy="215" r="150" fill="url(#sun)"/>

  <!-- The reflection of the cell on the water. -->
  <polygon points="812,790 512,1046 212,790 512,534" fill="url(#reflection)"/>

  <!-- The cell the peak stands on. -->
  <polygon points="812,560 662,370 362,370 212,560 362,750 662,750"
           fill="#f2ece0" stroke="#968e80" stroke-width="7"/>

  <!-- The lit face of the peak. -->
  <polygon points="362,690 518,170 590,399 518,690" fill="#e4bfb4"/>
  <!-- The shadowed face. -->
  <polygon points="518,170 662,690 518,690 590,399" fill="#bc8d84"/>
  <!-- Snow on the summit. -->
  <polygon points="518,170 590,399 562,233 486,266" fill="#f2ece0"/>

  <!-- Trees. -->
  <polygon points="300,613 267,700 333,700" fill="#c2cdb4"/>
  <polygon points="300,572 273,632 327,632" fill="#aebe9c"/>
  <polygon points="724,613 691,700 757,700" fill="#c2cdb4"/>
  <polygon points="724,572 697,632 751,632" fill="#aebe9c"/>
</svg>
"""


def main():
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    image = draw_icon()
    png = OUT_DIR / "appicon.png"
    image.save(png)
    print("wrote", png)

    svg = OUT_DIR / "appicon.svg"
    svg.write_text(SVG)
    print("wrote", svg)


if __name__ == "__main__":
    main()
