import type { ColorName } from "../types";

/**
 * ArkPeak's colour palette.
 *
 * This is the single source of truth for colour in the renderer. Everything
 * is a muted pastel, chosen to read as cozy rather than saturated.
 */

/** The colours the player can paint with. */
export interface ColorDef {
  /** The colour's name, as sent to and received from the backend. */
  readonly name: ColorName;
  /** The colour used for walls, roofs and other solid surfaces. */
  readonly solid: number;
  /** The colour used for floors, ground and other flat surfaces. */
  readonly flat: number;
  /** The colour used for trim, window frames and other details. */
  readonly trim: number;
}

/** The colours the player can choose, in palette order. */
export const COLORS: readonly ColorDef[] = [
  {
    name: "",
    solid: 0xe6ded0,
    flat: 0xd8cfbe,
    trim: 0xb9ae99,
  },
  {
    name: "sand",
    solid: 0xe8d5b0,
    flat: 0xdcc7a0,
    trim: 0xc0a97f,
  },
  {
    name: "cream",
    solid: 0xf2ece0,
    flat: 0xe7e0d1,
    trim: 0xc9c0ac,
  },
  {
    name: "rose",
    solid: 0xe4bfb4,
    flat: 0xd6ada3,
    trim: 0xbc8d84,
  },
  {
    name: "sage",
    solid: 0xc2cdb4,
    flat: 0xb2bda5,
    trim: 0x96a189,
  },
  {
    name: "slate",
    solid: 0xb4bcc4,
    flat: 0xa5adb6,
    trim: 0x8a929b,
  },
  {
    name: "terracotta",
    solid: 0xd79a76,
    flat: 0xc78a69,
    trim: 0xa96f53,
  },
] as const;

/** The colour used for the water. */
export const WATER_COLOR = 0x8fb0bd;

/** The colour used for the sky. */
export const SKY_COLOR = 0xcfdce0;

/** The colour of the sun light. */
export const SUN_COLOR = 0xfff2dc;

/** The colour of the ambient light. */
export const AMBIENT_COLOR = 0xa8bcc4;

/** The colour of the fog. */
export const FOG_COLOR = 0xc4d4d9;

/** The default colour, used when a cell has no explicit colour. */
export const DEFAULT_COLOR: ColorDef = COLORS[0];

/** Looks up a colour by name. */
export function colorByName(name: string): ColorDef {
  for (const color of COLORS) {
    if (color.name === name) {
      return color;
    }
  }
  return DEFAULT_COLOR;
}

/**
 * The colour of a prop's part.
 *
 * The props are not coloured by the player, so they have their own
 * colours. They are muted to match the rest of the town.
 */
export function propColor(kind: "body" | "detail", id: string): number {
  switch (id) {
    case "tree":
      return kind === "body" ? 0x8a6a4f : 0x7d9a6a;
    case "shrub":
      return kind === "body" ? 0x8fa378 : 0x8fa378;
    case "flower":
      return kind === "body" ? 0x7d9a6a : 0xdba7a7;
    case "lamp":
      return kind === "body" ? 0x6b6257 : 0xffe9bd;
    case "barrel":
      return kind === "body" ? 0xa98863 : 0xa98863;
    case "rock":
      return kind === "body" ? 0xa9a396 : 0xa9a396;
    default:
      return kind === "body" ? 0x9a9184 : 0xb9ae99;
  }
}
