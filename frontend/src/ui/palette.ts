import type { ColorName, TileType } from "../types";
import { COLORS, colorByName } from "../render/palette";

/**
 * The building palette.
 *
 * The tile buttons are built from the list the backend sends in
 * `TownSnapshot.tiles`, so adding a tile to the ruleset makes it appear here
 * without a frontend change.
 */
export class Palette {
  /** The tile the player is currently placing. */
  public tile: TileType = "wall";

  /** The colour the player is currently painting with. */
  public color: ColorName = "";

  private readonly tileButtons = new Map<string, HTMLButtonElement>();
  private readonly colorButtons = new Map<string, HTMLButtonElement>();

  constructor(
    container: HTMLElement,
    availableTiles: readonly string[],
  ) {
    this.buildTileGroup(container, availableTiles);
    this.buildColorGroup(container);
  }

  /** Builds the row of tile buttons. */
  private buildTileGroup(
    container: HTMLElement,
    availableTiles: readonly string[],
  ): void {
    const group = document.createElement("div");
    group.className = "palette-group";

    const label = document.createElement("p");
    label.className = "palette-label";
    label.textContent = "Build";
    group.appendChild(label);

    for (const tile of availableTiles) {
      const button = document.createElement("button");
      button.className = "palette-button";
      button.type = "button";
      button.textContent = tile;
      button.title = `Place a ${tile}`;
      button.addEventListener("click", () => {
        this.selectTile(tile);
      });
      group.appendChild(button);
      this.tileButtons.set(tile, button);
    }

    container.appendChild(group);
    this.selectTile(this.tile);
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
