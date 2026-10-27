import * as THREE from "three";

import { WATER_COLOR } from "../../render/palette";

/**
 * The animated water plane.
 *
 * The water is a single large plane with a vertex shader that gently wobbles
 * its surface. There is no ocean simulation: the point is a calm, moving
 * surface that catches the light.
 */
export class Water {
  /** The water plane. */
  public readonly mesh: THREE.Mesh;

  private readonly material: THREE.ShaderMaterial;
  private readonly geometry: THREE.PlaneGeometry;
  private time = 0;

  constructor(size = 400) {
    this.geometry = new THREE.PlaneGeometry(size, size, 64, 64);
    this.geometry.rotateX(-Math.PI / 2);

    this.material = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
        uColor: { value: new THREE.Color(WATER_COLOR) },
        uDeepColor: { value: new THREE.Color(WATER_COLOR).multiplyScalar(0.82) },
        uSunColor: { value: new THREE.Color(0xfff2dc) },
      },
      vertexShader: `
        uniform float uTime;
        varying float vWave;
        varying vec3 vWorldPosition;

        void main() {
          vec3 pos = position;

          // Two crossing wave trains give a gentle, irregular swell.
          float w1 = sin(pos.x * 0.18 + uTime * 0.7) * 0.5;
          float w2 = sin(pos.z * 0.23 - uTime * 0.5) * 0.35;
          float w3 = sin((pos.x + pos.z) * 0.11 + uTime * 0.9) * 0.25;

          pos.y += w1 + w2 + w3;
          vWave = w1 + w2 + w3;

          vec4 world = modelMatrix * vec4(pos, 1.0);
          vWorldPosition = world.xyz;
          gl_Position = projectionMatrix * viewMatrix * world;
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uDeepColor;
        uniform vec3 uSunColor;
        varying float vWave;
        varying vec3 vWorldPosition;

        void main() {
          // A soft gradient from the wave crest towards the trough.
          float t = clamp(vWave * 0.5 + 0.5, 0.0, 1.0);
          vec3 color = mix(uDeepColor, uColor, t);

          // A gentle highlight where the swell catches the light.
          float glint = smoothstep(0.55, 1.0, t);
          color = mix(color, uSunColor, glint * 0.18);

          gl_FragColor = vec4(color, 1.0);
        }
      `,
    });

    this.mesh = new THREE.Mesh(this.geometry, this.material);
    this.mesh.receiveShadow = true;
    this.mesh.position.y = -0.35;
  }

  /**
   * Advances the water animation.
   *
   * @param dt the time since the last frame, in seconds.
   */
  public update(dt: number): void {
    this.time += dt;
    this.material.uniforms.uTime.value = this.time;
  }
}
