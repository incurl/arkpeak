# Chapter 2 — Before Wave Function Collapse

---

## Why this chapter exists

It would be tidy to say: Oskar read Maxim Gumin's WFC paper, applied it to
towns, and made Townscaper.

That is not quite what happened, and the true story is more useful,
because it shows how the ideas evolved in his hands. The WFC repository's
credits line reads:

> Oskar Stålberg made an island generator that combines triangle and quad
> tiles and uses a custom observation heuristic that doesn't produce local
> minimums.

An *observation heuristic* is a WFC term, and it is the one part of the
algorithm Oskar did not take from the paper. Everything else — the
propagation, the constraint model — he had already worked out years
earlier, by hand, in a different way.

## The experiments, in order

Oskar's own Tumblr archive documents the progression. These are his own
posts, quoted from `oskarstalberg.tumblr.com`.

### 2013 — "Build your own little island"

> "Another interactive web demo. Build your own little island."
> — [post 81425316469](https://oskarstalberg.tumblr.com/post/81425316469/another-interactive-web-demo-build-your-own)

The linked demo is at `oskarstalberg.com/game/Island/Index.html`, and the
page title gives the game away: it was a **Unity Web Player** project, with
a page heading reading "Castle".

This is the ancestor of Townscaper, and it is already recognisably the
same idea: an island you can shape, that shapes itself around you.

### 2014 — "CityGenerator"

> "Interactive web demo. Made in 1 day in Construct 2. Runs in browser, try
> it out:"
> — [post 73038334145](https://oskarstalberg.tumblr.com/post/73038334145/interactive-web-demo-made-in-1-day-in-construct)

A city generator built in **one day** in Construct 2, running in a
browser. That is a remarkable constraint. It means the algorithm had to
be simple enough to write from scratch in a day, in a general-purpose
game engine, with no libraries.

### 2015 — "Polygonal Planet"

> "Polygonal Planet Project a study in tilesets, by Oskar Stålberg. Playable
> in your browser at: `oskarstalberg.com/game/planet/planet.html`"
> — [post 136461533291](https://oskarstalberg.tumblr.com/post/136461533291/polygonal-planet-project-a-study-in-tilesets-by)

A *study in tilesets*. This is the project where the tile vocabulary
became the subject in its own right rather than an implementation detail.

### And a much earlier one — "Brick Block"

> "I'm finally on twitter! Follow me at: https://twitter.com/OskSta To
> celebrate, brought Brick Block back to life! (back to being playable in
> Chrome, that is. Without downloading a plugin this time)"
> — [post 149376360741](https://oskarstalberg.tumblr.com/post/149376360741/im-finally-on-twitter-follow-me)

Brick Block was a **wave-based puzzle game** — the genre where you move a
crusher through a landscape of blocks that collapse under gravity. That is
not procedural generation at all; it is a physics puzzle.

But look at the shape of it: a grid of blocks, a rule that says which
blocks survive and which fall, and a chain reaction that propagates from
one place to the next. Brick Block is WFC's *propagation* phase, played as
a game, years before Oskar wrote a solver.

And then:

> "Vertex 3 finally dropped. On page 170, you can find my breakdown of
> Brick Block"
> — [post 132203650061](https://oskarstalberg.tumblr.com/post/132203650061/vertex-3-finally-dropped-on-page-170-you-can)

## What to take from this

Three lessons, all of which ArkPeak re-learns.

### Lesson 1: propagation is the interesting part

Brick Block is a propagation puzzle. CityGenerator is a propagation
generator. Townscaper is a propagation generator with better art.

The thing that makes a generated town feel *alive* is not the shape of the
tiles or the size of the vocabulary. It is that **placing one thing
changes what is possible next door**, and the player gets to watch that
happen.

If you are building a generator, build the propagation first. It is the
part that pays.

### Lesson 2: "one day in Construct 2" is a design constraint worth keeping

Oskar could have reached for a research-grade implementation. He did not,
and Townscaper is better for it.

ArkPeak takes the same constraint deliberately. The entire solver is
`internal/wfc/solver.go`, under 800 lines, with no dependencies beyond the
standard library. The rules are a JSON file. There is no code generation,
no plugin architecture, no framework.

This matters for more than tidiness. A generator you can read in an
afternoon is a generator you can *change* in an afternoon, and the whole
point of Townscaper-like games is that the player is constantly nudging
the rules into new configurations.

### Lesson 3: the tile vocabulary is a design object

"Polygonal Planet" was billed as "a study in tilesets" — the tiles were
the subject. Before you write the solver, decide what your tiles mean and
what they mean to each other.

In ArkPeak, that decision is the `town_rules.json` file, and it is the
only place the vocabulary exists. The solver never says "wall". It says
"this tile has connector `wall` on its north edge".

## The Island generator, specifically

The WFC repository's note about Oskar's island generator is worth reading
twice:

> combines triangle and quad tiles and uses a custom observation heuristic
> that doesn't produce local minimums

**Triangles and quads.** A square grid of squares has an awkward
property: its cells are aligned in rows and columns, and you can see the
alignment. Triangles and quads mixed together break the grid visually. A
quad next to a triangle has a diagonal edge, and the diagonals scatter the
eye.

**A custom observation heuristic.** The standard WFC heuristic picks the
cell with the lowest entropy. Oskar changed it. "Doesn't produce local
minimums" means: the standard heuristic can get *stuck*.

A **local minimum** is a configuration that is valid but boring — and in
Oskar's case, one that produces a visually poor result. The solver
satisfies the rules, but the rules alone did not guarantee a good-looking
island.

This is an important and underappreciated point, and Chapter 4 is devoted
to it. In short: the standard entropy heuristic optimises for
*constraint satisfaction*, but a good town needs *aesthetic* criteria that
the constraints do not express.

ArkPeak sidesteps the problem in a specific way. Its observation heuristic
is not entropy but **candidate count**:

```go
// SelectCell returns the unsettled cell with the fewest remaining
// candidates.
func (s *Solver) SelectCell() (CellKey, bool) {
	...
	n := c.count()   // popcount of the candidate bitmask
	if !found || n < bestCount {
		best = k
		bestCount = n
		found = true
	}
	...
}
```

This is the same *idea* as entropy — "most constrained first" — but
computed differently. Maxim Gumin's reference implementation offers both:

```csharp
double entropy = heuristic == Heuristic.Entropy ? entropies[i] : remainingValues;
if (remainingValues > 1 && entropy <= min)
```

The two agree often, and differ when candidates have very different
*weights*. Entropy accounts for weights; raw candidate count does not.

ArkPeak uses candidate count because it is cheaper (a popcount of a
`uint64` versus maintaining running sums of weights and weight-log-weights,
as the reference implementation does). For ArkPeak's tile weights — which
range from 2 to 30 — the two heuristics agree closely enough that the
difference does not show.

**This is a real simplification, and it is worth knowing about.** If you
wanted ArkPeak to look more like Townscaper's island generator, this is
the knob to turn: implement true Shannon entropy, and add whatever
aesthetic criterion Oskar uses to avoid his local minima.

## A note on sources

The Tumblr posts quoted above are Oskar Stålberg's own words, retrieved
from the `api/read` endpoint of `oskarstalberg.tumblr.com`. The WFC
repository notes are from Maxim Gumin's `README.md` and `Model.cs` at
<https://github.com/mxgmn/WaveFunctionCollapse>, which is the canonical
reference implementation and the closest thing the field has to a
primary source.

Oskar's more technical writing has historically lived on a personal blog
(`oskarsvante.se`) and in long-form magazine pieces. At the time of
writing, the blog domain no longer resolves and no archived copies were
available, so this book leans on the Tumblr archive, the WFC repository,
and the artefacts themselves — which is, in fairness, the more durable
record anyway. The games are the argument.

---

## What ArkPeak takes from this chapter

| From Oskar's earlier work | In ArkPeak |
|---|---|
| Keep the solver small and readable | `internal/wfc/solver.go`, under 800 lines, stdlib only |
| The tile vocabulary is the design | `town_rules.json` is the only place tiles are described |
| Propagation is the fun part | The solver's queue-based propagation is the core loop |
| Watch out for local minima | Chapter 4 covers this; ArkPeak's heuristic is a simplification |
| Break the visual grid | The hex grid plus per-cell jitter breaks up the lattice |

## Exercises

If you want to feel the algorithm rather than read about it:

1. **Change one weight.** Open `internal/town/rules/town_rules.json` and
   make `garden` weight 40 instead of 5. Restart the app. The town is now
   mostly gardens. The vocabulary is the design.

2. **Break one rule.** Remove `{ "a": "roof", "b": "roof", "axis": "hex" }`.
   Now two roofs cannot touch, so the solver will complain. That error is
   the rule failing, which is the clearest possible demonstration that
   the rules *are* the generator.

3. **Read `Propagate()`** in `internal/wfc/solver.go` and find the moment
   a choice is forbidden. Everything else in the algorithm exists to get
   there.

---

*Previous: [Chapter 1 — Townscaper](01-townscaper.md) · Next: [Chapter 3 — Wave Function Collapse from the ground up](03-wfc.md)*
