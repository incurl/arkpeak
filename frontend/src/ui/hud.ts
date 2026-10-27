/**
 * The heads-up display: the small status panel in the corner.
 */
export class Hud {
  private readonly status: HTMLElement | null;

  /** The handle of the animation that cycles the generating indicator. */
  private spinner: number | null = null;

  constructor() {
    this.status = document.getElementById("hud-status");
  }

  /** Updates the status line. */
  public setStatus(text: string): void {
    this.stopSpinner();
    if (this.status !== null) {
      this.status.textContent = text;
    }
  }

  /** Shows an error in the status line. */
  public setError(text: string): void {
    this.stopSpinner();
    this.setStatus(`Error: ${text}`);
  }

  /**
   * Shows a working indicator until setStatus is called.
   *
   * The backend solves the whole town before the first frame is drawn,
   * which takes a couple of seconds. Without an animation the window
   * looks frozen, so the indicator makes the wait legible.
   */
  public showWorking(message: string): void {
    if (this.status === null) {
      return;
    }

    const frames = ["·", "··", "···", "····"];
    let index = 0;

    this.status.textContent = `${message} ${frames[0]}`;
    this.spinner = window.setInterval(() => {
      index = (index + 1) % frames.length;
      if (this.status !== null) {
        this.status.textContent = `${message} ${frames[index]}`;
      }
    }, 280);
  }

  /** Stops the working indicator. */
  private stopSpinner(): void {
    if (this.spinner !== null) {
      window.clearInterval(this.spinner);
      this.spinner = null;
    }
  }
}
