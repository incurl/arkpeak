/**
 * The heads-up display: the small status panel in the corner.
 */
export class Hud {
  private readonly status: HTMLElement | null;

  constructor() {
    this.status = document.getElementById("hud-status");
  }

  /** Updates the status line. */
  public setStatus(text: string): void {
    if (this.status !== null) {
      this.status.textContent = text;
    }
  }

  /** Shows an error in the status line. */
  public setError(text: string): void {
    this.setStatus(`Error: ${text}`);
  }
}
