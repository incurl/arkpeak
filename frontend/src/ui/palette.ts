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
 * the storey above. The palette therefore shows one group per layer, and
 * the player picks the layer they are building on. The active layer decides
 * where a click lands, so the choice is always the player's and never
 * changes under their fingers.
 */
export class Palette {
  /** The tile the player is currently placing. */
  public tile: TileType = "wall";

  /** The colour the player is currently painting with. */
  public color: ColorName = "";

  /** The layer the player is currently building on. */
  public layer = 0;

  private readonly tileButtons = new Map<string, HTMLButtonElement>();
  private readonly colorButtons = new Map<string, HTMLButtonElement>();

  /** The layer each tile belongs to, or -1 for a tile valid on any layer. */
  private readonly tileLayers = new Map<string, number>();

  /** The tiles grouped by the layer they belong to. */
  private readonly byLayer = new Map<number, string[]>();

  /** The layer groups, so the right one can be highlighted. */
  private readonly layerGroups = new Map<number, HTMLElement>();

  /** The labels that switch the active layer. */
  private readonly layerLabels = new Map<number, HTMLElement>();

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

    // Group the tiles by layer.
    const layers = new Set<number>();
    for (const info of tileInfo) {
      layers.add(info.layer);
    }
    if (layers.size === 0) {
      layers.add(0);
    }

    const sorted = [...layers].sort((a, b) => a - b);
    for (const layer of sorted) {
      const tiles = availableTiles.filter(
        (tile) => (this.tileLayers.get(tile) ?? -1) === layer,
      );
      if (tiles.length === 0) {
        continue;
      }

      const layerGroup = document.createElement("div");
      layerGroup.className = "palette-layer";
      layerGroup.dataset.layer = String(layer);

      const layerLabel = document.createElement("button");
      layerLabel.className = "palette-sublabel";
      layerLabel.type = "button";
      layerLabel.textContent = layer === 0 ? "Ground" : `Storey ${layer}`;
      layerLabel.addEventListener("click", () => {
        this.showLayer(layer);
      });
      layerGroup.appendChild(layerLabel);
      this.layerLabels.set(layer, layerLabel);

      for (const tile of tiles) {
        this.tileButtons.set(tile, this.buildTileButton(layerGroup, tile, layer));
        const list = this.byLayer.get(layer);
        if (list === undefined) {
          this.byLayer.set(layer, [tile]);
        } else {
          list.push(tile);
        }
      }

      this.layerGroups.set(layer, layerGroup);
      group.appendChild(layerGroup);
    }

    container.appendChild(group);
    this.selectTile(this.tile);
    this.showLayer(this.layer);
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
    button.title = `Place a ${tile}`;
    button.dataset.layer = String(layer);
    button.addEventListener("click", () => {
      this.selectTile(tile);
    });
    group.appendChild(button);
    return button;
  }

  /**
   * Makes the given layer the active one.
   *
   * The active layer decides where a click lands. The player's tile is not
   * changed: if it does not belong to the new layer, the placement is
   * refused with an explanation rather than being silently swapped.
   */
  public showLayer(layer: number): void {
    this.layer = layer;

    for (const [groupLayer, group] of this.layerGroups) {
      group.classList.toggle("is-active", groupLayer === layer);
    }
    for (const [labelLayer, label] of this.layerLabels) {
      label.classList.toggle("is-active", labelLayer === layer);
    }

    for (const [name, button] of this.tileButtons) {
      button.disabled = (this.tileLayers.get(name) ?? -1) !== layer;
    }
  }

  /**
   * Reports whether the selected tile may be placed on the given layer.
   */
  public tileFitsLayer(layer: number): boolean {
    const tileLayer = this.tileLayers.get(this.tile);
    if (tileLayer === undefined) {
      return false;
    }
    return tileLayer === -1 || tileLayer === layer;
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
