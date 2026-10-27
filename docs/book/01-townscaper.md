# Chapter 1 — Townscaper: the game that is not a game

---

## What it is

**Townscaper** was made by Oskar Stålberg and released in 2021. It looks
like this: a soft, pastel island floating in a white void. You click a
tile. Buildings appear. You click another tile. More buildings appear,
arranged around what you just placed, connected sensibly to what was
already there.

You can drag across the screen and paint a whole neighbourhood. You can
zoom in until a single window fills the view. You can zoom out until the
whole island is a few pixels across.

There is no score. There is no timer. There is no resource to manage and
nothing to lose. You cannot win and you cannot lose. You cannot even
really "play" it in the way you would describe playing a strategy game.

And yet it sold enormously, and it has become a reference point for
procedural generation so complete that "is it the new Townscaper?" is a
compliment you can pay a project.

## Why it is interesting

Most procedural generators make a **maze** or a **dungeon**: a space full
of stuff, generated for the player to traverse and fight through. The
generated thing exists to be *consumed*.

Townscaper inverts that. The generated thing is the entire point. The town
is not a level; it is the artwork. The pleasure is not in solving or
conquering the town but in watching it *resolve* — in the moment when you
place a block and the algorithm decides, in a fraction of a second, that
this should become a house with a pitched roof, and that the house next
door needs a chimney, and the chimney next to *that* needs to be taller.

Oskar calls this "the joy of procedural generation" and the pleasure is
genuinely in the *process*, not the product. Many players describe playing
Townscaper as relaxing in a way that is unusual for a computer game. There
is nothing to fail at, so there is no tension.

> The joy of Townscaper is not in the town you end up with. It is in the
> town arriving.

## What is actually happening under the cursor

This is the part worth being precise about, because it is the part ArkPeak
copies.

Townscaper is **not** a traditional city builder where you place a
building and it stays where you put it. It is not SimCity. In Townscaper:

- You click a spot on the island.
- The click does **not** put a specific building there. It puts a *tile* —
  a coloured block, one of a small set.
- The generator then re-runs over the affected area, and decides what
  should actually be there, consistent with the tile you placed and with
  everything else nearby.

So your click is a **constraint**, not a command. You are not saying "put a
house here". You are saying "there should be something of this kind here,
and the generator will work out what."

This is the single most important idea to carry into the rest of the
book, and it is why ArkPeak's backend is called `PlaceConstraint` rather
than `PlaceBuilding`.

### The tiles

Townscaper's tile set is small and abstract. Coloured squares —
essentially a palette of "wall", "roof", "edge", "tree", "water", "path"
and a handful of others — that you paint onto the grid. They are not
buildings. They are more like *notes in a musical score*: they say
something about the local character, and the generator composes the actual
music.

The vocabulary is deliberately tiny. Oskar's genius was in realising that a
very small vocabulary, combined with a very good solver, produces
enormous variety — and that the variety comes from the *arrangement*, not
from the size of the vocabulary.

## The island itself

The island is generated before you touch anything. It has a coastline, a
shape, a certain distribution of land. You can regenerate it, and the
shape changes, but the *character* is stable: there are always bays and
promontories, never a square.

Oskar described the island as generated with a form of WFC that combines
triangle and quad tiles — from the WFC repository's credits, he "made an
island generator that combines triangle and quad tiles and uses a custom
observation heuristic that doesn't produce local minimums."

That phrase — *doesn't produce local minimums* — is the densest technical
sentence in this chapter, and Chapter 4 is entirely about what it means.

## Why "cozy" is a technical decision

The palette matters more than it looks. Look at ArkPeak's colours, in
`frontend/src/render/palette.ts`:

```ts
export const COLORS: readonly ColorDef[] = [
  { name: "",          solid: 0xe6ded0, flat: 0xd8cfbe, trim: 0xb9ae99 },
  { name: "sand",      solid: 0xe8d5b0, flat: 0xdcc7a0, trim: 0xc0a97f },
  { name: "cream",     solid: 0xf2ece0, flat: 0xe7e0d1, trim: 0xc9c0ac },
  { name: "rose",      solid: 0xe4bfb4, flat: 0xd6ada3, trim: 0xbc8d84 },
  { name: "sage",      solid: 0xc2cdb4, flat: 0xb2bda5, trim: 0x96a189 },
  { name: "slate",     solid: 0xb4bcc4, flat: 0xa5adb6, trim: 0x8a929b },
  { name: "terracotta", solid: 0xd79a76, flat: 0xc78a69, trim: 0xa96f53 },
] as const;
```

Every one of these is a **desaturated pastel**. None is a pure hue. There
is no red, no bright green, no primary. The palette is doing specific
work:

- **Low saturation** means that any two neighbouring buildings look
  harmonious, whatever colours they are. The player can make an
  aggressively ugly town, and it will still be *cozy*.
- **Warm and cool are balanced.** The town has sandy warmth and slate
  coolness, and it is legitimate for a building to be either.
- **The trim colour is darker than the solid.** This is what makes the
  procedural geometry legible. Roof edges, window frames and garden
  kerbs are drawn in the trim colour, so you can read a building's
  *structure* even when every surface is the same pastel.

The last point is the one people miss. Cozy is not only a colour
decision; it is a *rendering* decision about how much detail to show and
in what colour.

## What Townscaper is not

It is worth being clear about the negative space, because ArkPeak's
scope is shaped by it:

- **It is not a simulation.** Nothing in Townscaper has an opinion about
  whether a town is *good*. There is no economy, no traffic, no
  happiness. The buildings do not need anything.
- **It is not a puzzle.** There is nothing to work out. The generator
  always succeeds.
- **It does not fail.** You cannot break it. This is a design constraint
  that ArkPeak also takes seriously — see Chapter 10, where a placement
  that *would* have failed turns out to be a bug.

## The lineage

Oskar did not invent WFC, but he was one of its most visible
practitioners, and Townscaper is the reason WFC is known outside of
technical circles. Maxim Gumin's WFC repository credits Townscaper
explicitly in its list of games that use the algorithm, alongside *Bad
North* and *Caves of Qud*.

What Townscaper added was not a new algorithm. It was the demonstration
that a well-tuned solver, a tiny vocabulary and a good renderer could turn
a mathematical constraint problem into something people would pay money
for and play for hours without any goal.

That is the lesson ArkPeak is built on, and it is the reason this book
exists.

---

## What ArkPeak takes from this chapter

| From Townscaper | In ArkPeak |
|---|---|
| Clicking places a *tile*, not a building | `PlaceConstraint(q, r, layer, tile, color)` |
| The tile is a constraint, not a command | The solver decides what actually goes there |
| A tiny tile vocabulary | 11 tiles in `town_rules.json` |
| Desaturated pastel palette | `COLORS` in `frontend/src/render/palette.ts` |
| Structure legible in a trim colour | `trim` in every `ColorDef` |
| Regenerating gives a new island | `NewTown(seed)` |
| Nothing to fail | Placement errors are explained, never fatal |

## What ArkPeak does differently

- **Townscaper's island is generated with triangle and quad tiles.**
  ArkPeak's is a hexagon of hex cells — simpler to reason about and to
  draw, and it matches the solver's own coordinate system.
- **ArkPeak's tiles have more structure.** Townscaper's are close to
  pure colour. ArkPeak's have connectors on all six sides, which is what
  makes neighbour-aware geometry possible at all (Chapter 9).
- **ArkPeak has two layers.** Ground and storey. Townscaper is
  effectively flat in this respect.

---

*Previous: [Preface](00-preface.md) · Next: [Chapter 2 — Before Wave Function Collapse](02-before-wfc.md)*
