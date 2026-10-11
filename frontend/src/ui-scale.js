const UI_DESIGN_WIDTH = 1360;
const UI_DESIGN_HEIGHT = 860;
const UI_MIN_SCALE = 0.72;
const UI_MAX_SCALE = 1.25;

function updateUiScale() {
  const width = Math.max(window.innerWidth || UI_DESIGN_WIDTH, 1);
  const height = Math.max(window.innerHeight || UI_DESIGN_HEIGHT, 1);
  const screenWidth = Math.max(window.screen?.availWidth || width, 1);
  const screenHeight = Math.max(window.screen?.availHeight || height, 1);
  const rawScale = Math.min(
    width / UI_DESIGN_WIDTH,
    height / UI_DESIGN_HEIGHT,
    screenWidth / UI_DESIGN_WIDTH,
    screenHeight / UI_DESIGN_HEIGHT
  );
  const scale = Math.min(UI_MAX_SCALE, Math.max(UI_MIN_SCALE, rawScale));
  const root = document.documentElement;
  root.style.setProperty("--ui-scale", String(scale));
  root.style.setProperty("--ui-viewport-width", `${width / scale}px`);
  root.style.setProperty("--ui-viewport-height", `${height / scale}px`);
}

updateUiScale();
window.addEventListener("resize", updateUiScale, { passive: true });
