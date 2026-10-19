# Mobile UI polish (guest frontend)
Status: IN PROGRESS. Stacked branches from `develop` (6bb1835): `fix/mobile-viewer` → `feat/mobile-shell` → `feat/pinned-restyle` → `feat/gallery-motion` → `perf/guest-bundle` → `chore/static-cache`; only the last one is merged.
Checks: `cd frontend && npm test && npx svelte-check && npm run build` (budget S12); headless Chromium mobile walk (360×740, 390×844, 740×360) with a mock API; real phone walk (runbook T9) before merge.

## Specs
- **S1** The download button label stays «Descargar» (L2: "el cambio de «Descargar original» → «Descargar» se mantiene para reducir el tamano de los botones que ayuda mucho para el F1").
- **S2** Viewer controls fit in one row of fixed height at 360 px wide, including the admin «Ocultar» action; like/hide error messages never move the image or the controls (F1).
- **S3** The viewer adapts to any media format: "hacer alguna medida de que aunque se modifique no salga de ciertos parametros o que la UI se adapte al formato de la imagen" (L2). Portrait, landscape, panoramic, square photos and videos are shown whole (contain) inside a fixed media area between the top bar and the controls; they never cover the controls and the controls never move between items. A phone in landscape puts the controls in a side column so the media keeps the height.
- **S4** The phone back button closes an open photo without leaving the gallery; closing with ✕ or Escape leaves no extra history entry (F2).
- **S5** Swiping between photos ignores two-finger (pinch) gestures, mostly vertical drags and drags on the video controls strip (F3).
- **S6** The gallery behind an open viewer does not scroll, and closing the viewer keeps the gallery scroll position (F4).
- **S7** Gallery photos and pinned images open in the same viewer shell (one implementation).
- **S8** Bottom navigation fixed at the bottom "como en varias apps conocidas como instagram, youtube o X" (L2), with «Recientes», «Más gustadas» and upload; upload progress stays visible in a compact form.
- **S9** «Fijados» gets a different, non-invasive look: "que cambi un poco la estetica/formato para que se vea diferente, pero que tampoco se muy invasivo (no me gustaba que hubiera un boton con la palabra desplegar/plegar , mejor un icono)" (L2). The collapse toggle is an icon with an accessible label; the collapsed state is still remembered per device.
- **S10** Livelier gallery: images fade in on load, like heart animates on the tile, active tab indicator animates, viewer opens with a short transition; tile like badge is easier to hit and its count is announced to screen readers (F5, F8). Square tiles stay (L2: "el F6 no me importa ... queda bien para que se vea la galeria simetrica").
- **S11** Guests do not download the admin code; the viewer preloads only the next image.
- **S12** Performance budget: guest JS ≤ 35.5 KB gzip (baseline 102.4 KB / 35.5 KB gzip), CSS ≤ 4 KB gzip (baseline 2.75 KB); animations only on `transform`/`opacity`; no `backdrop-filter` over the grid; no new dependencies; no per-tile Svelte transitions; everything honours `prefers-reduced-motion`; windowing keeps ≤ 150 mounted tiles.
- **S13** Hashed static assets (`/assets/*`) are cached as immutable by Caddy; `index.html` is always revalidated.

Dropped: F6 tile aspect change (L2, squares stay); F7 notch/safe areas (L2: "me parece un poco irrelevante").

## Tasks
| ID | Specs | Route | Branch | Commit |
|---|---|---|---|---|
| T1 | S1-S7 | inline, test-first for pure logic | `fix/mobile-viewer` | pending |
| T2 | S8, S12 | inline | `feat/mobile-shell` | pending |
| T3 | S9, S12 | inline | `feat/pinned-restyle` | pending |
| T4 | S10, S12 | inline | `feat/gallery-motion` | pending |
| T5 | S11, S12 | inline | `perf/guest-bundle` | pending |
| T6 | S13 | inline · Risk: item 5 (Caddy delivery config) → own commit | `chore/static-cache` | pending |

## Log
- **L1** (user, original request): "Ahora que ya tengo la app funcional, la parte de back mas o menos cerrada quiero que cojas y hagas una buena repasada al front para cumplir con todas las buenas practicas para una app enfocada a movil, ya que por lo que he visto al ser una galeria y tener diferentes formatos hay imagenes que solapan/ocultan parte de la UI y eso puede ser incomodo. Ademas de toda esta parte ciertamente funcional quiero que le des un repaso mas a nivel estetico (la paleta de colores creo que encaja perfecto con el concepto, pero ciertas animaciones cietos layouts se me quedan sosos), por ejemplo la parte de fijados me gusta pero veo que no esta del todo bien integrado al igual que el tema de la seccion de galeria y megustas que creo que se podri integrar mas a un formatod de movil. Esta primera accion quiero que sea a nivel de planificacion y analisis, ya que no quiero que afecte al rendimiento de la app que es el posible cuello de botella de la viabilidad del proyecto. Como siempre recuerda utilizar las herramientas de gentle ai"
- **L2** (user, plan decisions): "Antes de comenzar quiero tener en cuenta unos cambios para el plan, lo primero que el cambio de «Descargar original» → «Descargar» se mantiene para reducir el tamano de los botones que ayuda mucho para el F1, el F6 no me importa mucho el que se recorte de hecho me parece que queda bien para que se vea la galeria simetrica, pero lo que no me convence tanto es el tema de que al amplairla al tener formatos diferentes se pueda llegar a ver mal o no encajar del todo con la UI por eso queria hacer alguna medida de que aunque se modifique no salga de ciertos parametros o que la UI se adapte al formato de la imagen. El punt F7 me parece un poco irrelevante porque con la barra de arriba del navegador casi me la pela el notch. En cuanto a las decisiones me parece correcto que se fije abajo como en varias apps conocidas como instagram, youtube o X. El formatod de la galeria como he dicho se mantiene. De la parte de fijados me parece correcto, lo que busco es que cambi un poco la estetica/formato para que se vea diferente, pero que tampoco se muy invasivo (no me gustaba que hubiera un boton con la palabra desplegar/plegar , mejor un icono). La ultima parte de la cache no la he entendido muy bien pero me gusta"
- **L3** (parent, analysis evidence): baseline build JS 102.4 KB (35.5 KB gzip), CSS 10.9 KB (2.75 KB gzip), font woff2 23 KB. Tiles open through `<a href="#/foto/id">` (a pushed entry), navigation inside the viewer uses `replaceState`, ✕ close uses `replaceState` → a stale duplicate entry; a deep link opened directly has no entry to go back to.
