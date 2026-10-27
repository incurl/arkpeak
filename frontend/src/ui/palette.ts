import type { ColorName, TileInfo, TileType } from "../types";
import { COLORS, colorByName } from "../render/palette";

/**
 * The building palette.
 *
 * The tile buttons are built from the list the backend sends in
 * `TownSnapshot.tiles`, so adding a tile to the ruleset makes it appear here
 * without a frontend change.
 *
 * Each tile belongs to a layer: a wall sits on the ground, a roof sits on
 * the storey above. The palette therefore only offers the tiles that are
 * valid on the layer the player is pointing at, which stops the backend
 * from rejecting a placement the player could not have known was invalid.
 */
export class Palette {
  /** The tile the player is currently placing. */
  public tile: TileType = "wall";

  /** The colour the player is currently painting with. */
  public color: ColorName = "";

  private readonly tileButtons = new Map<string, HTMLButtonElement>();
  private readonly colorButtons = new Map<string, HTMLButtonElement>();

  /** The layer each tile belongs to, or -1 for a tile valid on any layer. */
  private readonly tileLayers = new Map<string, number>();

  /** The tiles grouped by the layer they belong to. */
  private readonly byLayer = new Map<number, string[]>();

  constructor(
    container: HTMLElement,
    availableTiles: readonly string[],
    tileInfo: readonly TileInfo[] = [],
  ) {
    this.buildTileGroup(container, availableTiles, tileInfo);
    this.buildColorGroup(container);
  }

  /** Builds the row of tile buttons. */
  private buildTileGroup(
    container: HTMLElement,
    availableTiles: readonly string[],
    tileInfo: readonly TileInfo[],
  ): void {
    const group = document.createElement("div");
    group.className = "palette-group";

    const label = document.createElement("p");
    label.className = "palette-label";
    label.textContent = "Build";
    group.appendChild(label);

    // Record which layer each tile belongs to.
    for (const info of tileInfo) {
      this.tileLayers.set(info.id, info.layer);
    }

    for (const tile of availableTiles) {
      const layer = this.tileLayers.get(tile) ?? -1;
      this.tileButtons.set(tile, this.buildTileButton(group, tile, layer));

      const list = this.byLayer.get(layer);
      if (list === undefined) {
        this.byLayer.set(layer, [tile]);
      } else {
        list.push(tile);
      }
    }

    container.appendChild(group);
    this.selectTile(this.tile);
  }

  /** Builds a single tile button. */
  private buildTileButton(
    group: HTMLElement,
    tile: string,
    layer: number,
  ): HTMLButtonElement {
    const button = document.createElement("button");
    button.className = "palette-button";
    button.type = "button";
    button.textContent = tile;
    button.title =
      layer < 0
        ? `Place a ${tile}`
        : `Place a ${tile} on layer ${layer}`;
    button.dataset.layer = String(layer);
    button.addEventListener("click", () => {
      this.selectTile(tile);
    });
    group.appendChild(button);
    return button;
  }

  /**
   * Restricts the palette to the tiles valid on the given layer.
   *
   * A tile that is valid on every layer stays available everywhere. The
   * player's selection is kept if it is still valid, and otherwise falls
   * back to the first tile that is.
   */
  public setLayer(layer: number): void {
    const valid = this.tilesForLayer(layer);

    for (const [name, button] of this.tileButtons) {
      button.classList.toggle("is-hidden", !valid.includes(name));
      button.disabled = !valid.includes(name);
    }

    if (!valid.includes(this.tile)) {
      const fallback = valid[0];
      if (fallback !== undefined) {
        this.selectTile(fallback);
      }
    }
  }

  /** The tiles that may be placed on the given layer. */
  public tilesForLayer(layer: number): string[] {
    const specific = this.byLayer.get(layer) ?? [];
    const any = this.byLayer.get(-1) ?? [];
    return [...specific, ...any];
  }

  /** Builds the row of colour swatches. */
  private buildColorGroup(container: HTMLElement): void {
    const group = document.createElement("div");
    group.className = "palette-group";

    const label = document.createElement("p");
    label.className = "palette-label";
    label.textContent = "Colour";
    group.appendChild(label);

    for (const color of COLORS) {
      const button = document.createElement("button");
      button.className = "palette-swatch";
      button.type = "button";
      button.title = color.name === "" ? "Default colour" : color.name;
      button.style.background = `#${color.solid.toString(16).padStart(6, "0")}`;
      button.addEventListener("click", () => {
        this.selectColor(color.name);
      });
      group.appendChild(button);
      this.colorButtons.set(color.name, button);
    }

    container.appendChild(group);
    this.selectColor(this.color);
  }

  /** Selects the tile to place. */
  public selectTile(tile: string): void {
    this.tile = tile as TileType;
    for (const [name, button] of this.tileButtons) {
      button.classList.toggle("is-active", name === tile);
    }
  }

  /** Selects the colour to paint with. */
  public selectColor(color: ColorName): void {
    this.color = color;
    for (const [name, button] of this.colorButtons) {
      button.classList.toggle("is-active", name === color);
    }
  }

  /** The colour definition matching the selected colour. */
  public get colorDefinition(): ReturnType<typeof colorByName> {
    return colorByName(this.color);
  }
}
