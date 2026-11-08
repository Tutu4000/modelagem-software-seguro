// The 3D view of a part: three.js, rendered only when something changes, with
// mouse and touch orbit. Loaded on demand, so the rest of the app stays light.
import { useEffect, useRef } from "react";
import {
  AmbientLight,
  Box3,
  BufferAttribute,
  BufferGeometry,
  Color,
  DirectionalLight,
  EdgesGeometry,
  GridHelper,
  LineBasicMaterial,
  LineSegments,
  Mesh as ThreeMesh,
  MeshStandardMaterial,
  PerspectiveCamera,
  Scene,
  Vector3,
  WebGLRenderer,
} from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";

import { dimensions } from "./dimensions";
import type { Mesh } from "./stl";
import styles from "./StlViewer.module.css";

export interface StlViewerProps {
  mesh: Mesh;
  /** The accessible description of the picture. */
  label: string;
}

export function StlViewer({ mesh, label }: StlViewerProps) {
  const host = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const reset = useRef<() => void>(() => undefined);
  const spin = useRef<() => void>(() => undefined);

  useEffect(() => {
    const el = canvas.current;
    const box = host.current;
    if (el === null || box === null) {
      return;
    }
    const renderer = new WebGLRenderer({ canvas: el, antialias: true, alpha: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    const scene = new Scene();
    scene.add(new AmbientLight(0xffffff, 0.7));
    const sun = new DirectionalLight(0xffffff, 1.6);
    sun.position.set(1, 2, 3);
    scene.add(sun);

    const geometry = new BufferGeometry();
    geometry.setAttribute("position", new BufferAttribute(mesh.positions, 3));
    geometry.setAttribute("normal", new BufferAttribute(mesh.normals, 3));
    geometry.rotateX(-Math.PI / 2); // STL is z-up; the scene is y-up.
    geometry.computeBoundingBox();
    const bounds = geometry.boundingBox ?? new Box3();
    const center = bounds.getCenter(new Vector3());
    geometry.translate(-center.x, -bounds.min.y, -center.z);

    const part = new ThreeMesh(geometry, new MeshStandardMaterial({ color: new Color("#c9ced6"), metalness: 0.1, roughness: 0.6 }));
    const edges = new LineSegments(new EdgesGeometry(geometry, 30), new LineBasicMaterial({ color: new Color("#0b1f3a") }));
    scene.add(part, edges);

    const size = bounds.getSize(new Vector3());
    const extent = Math.max(size.x, size.y, size.z, 1);
    const grid = new GridHelper(extent * 3, 12, new Color("#9aa3b2"), new Color("#dde1e8"));
    scene.add(grid);

    const camera = new PerspectiveCamera(40, 1, extent / 100, extent * 100);
    const controls = new OrbitControls(camera, el);
    controls.enableDamping = false;
    const home = () => {
      camera.position.set(extent * 1.6, extent * 1.2, extent * 1.8);
      controls.target.set(0, size.y / 2, 0);
      controls.update();
    };

    const render = () => { renderer.render(scene, camera); };
    const resize = () => {
      const { width, height } = box.getBoundingClientRect();
      renderer.setSize(width, height, false);
      camera.aspect = width / Math.max(height, 1);
      camera.updateProjectionMatrix();
      render();
    };
    controls.addEventListener("change", render);
    const observer = new ResizeObserver(resize);
    observer.observe(box);
    home();
    resize();

    reset.current = () => {
      home();
      render();
    };
    // A quarter turn on request, never on its own (prefers-reduced-motion).
    spin.current = () => {
      const angle = Math.PI / 2;
      const { x, z } = camera.position;
      camera.position.x = x * Math.cos(angle) - z * Math.sin(angle);
      camera.position.z = x * Math.sin(angle) + z * Math.cos(angle);
      controls.update();
      render();
    };

    return () => {
      observer.disconnect();
      controls.dispose();
      geometry.dispose();
      renderer.dispose();
    };
  }, [mesh]);

  return (
    <div ref={host} className={styles.viewer}>
      <canvas ref={canvas} className={styles.canvas} role="img" aria-label={`${label}: ${dimensions(mesh.bbox)}`} />
      <span className={styles.dims} data-testid="dimensions">
        {dimensions(mesh.bbox)}
      </span>
      <div className={styles.tools}>
        <button type="button" onClick={() => { spin.current(); }}>
          Girar 90°
        </button>
        <button type="button" onClick={() => { reset.current(); }}>
          Vista inicial
        </button>
      </div>
    </div>
  );
}
