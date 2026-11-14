# 0031. The Memory Graph in 2D WebGL, Then Summaries for Millions
Date: 2026-11-14 · Status: Accepted (step 1 built)

## Context
The dashboard drew the graph in 3D (three.js, react-force-graph-3d) with the
newest 300 memories. It was slow beyond a few hundred nodes, hard to read,
and offered no way to find anything in it. We want the graph to stay useful
for one agent with 10 million memories and for an organisation with
10,000 users × 1,000 memories each.

## Decision
No browser can draw 10 million points as dots, and no person can read them.
So the graph shows a capped view, and a summary when there is more.

1. **Rendering (built):** sigma.js + graphology (MIT), WebGL in 2D.
   - ForceAtlas2 runs in a worker, so the page stays responsive.
   - People and companies are labelled. Hovering or selecting lights up a
     node's neighbourhood and fades the rest (mixed towards the background,
     not transparent; WebGL washes out transparent nodes).
   - The toolbar has search (Enter jumps to the best match), a size picker
     (300 / 600 / 1,000; 1,000 is the server's maximum), zoom and fit.
   - Hover labels follow the theme.
   - three.js and react-force-graph-3d are removed from the dashboard.
   - **Look** (chosen from five directions):
     - **Light is Studio Mono:** neutral greys at rest, and the accent
       only on the focus, its first ring and search hits.
     - **Dark is Observatory:** a deep background, white names, a soft
       glow on the labelled names and the focus, and faint hazes behind
       the three biggest clusters (a 2D canvas under the WebGL layers).
     - **Colour by:** Calm (the default), Clusters (three accessible
       colours) or Types.
     - **Faded colours are solid,** mixed into the background: WebGL adds
       transparent colours onto dark backgrounds, so they would glare.
       Tokens like `#fff` are expanded to 6-digit hex before mixing.
2. **Filters (next):** date range, memory type, person or company, and
   conversation, applied on the server, plus growing the graph outward from
   a chosen node one ring at a time.
3. **Summaries:** a background job groups memories into clusters by the
   people, companies and topics they mention, incrementally, and stores
   them.
   - The graph opens on clusters, sized by count and linked by
     co-occurrence, then drills down to memories a page at a time.
   - It works for one agent and for a whole organisation (owners).
4. **Scale target:** every graph request reads indexed tables and returns
   at most a few thousand items, whatever the total. This will be checked
   with a load test of 10 million seeded memories.

## Layout (2026-11-14)
Louvain runs before the layout and each community starts around its own
point on a circle, so ForceAtlas2 settles into separate groups. The three
coloured clusters are named above their centre. On hover or selection only
people and companies are labelled; memories light up and the side card lists
them.

## Known issues
- The entity extractor (`extract.Entities`) tags some non-names:
  sentence-start verbs ("emailed diego ruiz") and topic words ("wants",
  "series"). Search uses it too, so it gets its own change with the search
  quality tests. Until then the graph filters its people and companies
  itself (2026-11-14): a tag becomes a node only if some memory writes it
  capitalised mid-sentence, and job titles, departments, funding rounds,
  months and days are skipped. This works on tags already stored, with no
  backfill. Names written only in lowercase ("met ana at stripe") drop out.
- Switching the theme while a node is selected leaves the rest undimmed
  until the next click.
