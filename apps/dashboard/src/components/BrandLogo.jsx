import {
  siAirtable, siAsana, siBitbucket, siBuildkite, siCircleci, siClickup, siCoda, siConfluence, siDatadog,
  siDiscord, siFigma, siGithub, siGitlab, siGrafana, siHubspot, siIntercom, siJira, siLinear, siNetlify,
  siNewrelic, siNotion, siOpsgenie, siOutline, siPagerduty, siPlane, siPostman, siRender, siSentry,
  siShortcut, siSnyk, siSonarqubecloud, siSupabase, siTodoist, siTrello, siVercel, siZendesk, siZotero,
} from "simple-icons";
import close from "../assets/logos/close.png";
import freshdesk from "../assets/logos/freshdesk.png";
import front from "../assets/logos/front.jpg";
import gong from "../assets/logos/gong.png";
import gorgias from "../assets/logos/gorgias.png";
import harvest from "../assets/logos/harvest.png";
import monday from "../assets/logos/monday.png";
import pipedrive from "../assets/logos/pipedrive.png";
import raindrop from "../assets/logos/raindrop.png";
import readwise from "../assets/logos/readwise.png";
import wrike from "../assets/logos/wrike.png";
import youtrack from "../assets/logos/youtrack.png";

// Brand marks from Simple Icons (CC0), bundled so the dashboard loads no
// third-party images. Brands Simple Icons no longer carries use the
// service's own site icon, saved in assets/logos.
const ICONS = {
  airtable: siAirtable, asana: siAsana, bitbucket: siBitbucket, buildkite: siBuildkite, circleci: siCircleci,
  clickup: siClickup, coda: siCoda, confluence: siConfluence, datadog: siDatadog, discord: siDiscord,
  figma: siFigma, github: siGithub, gitlab: siGitlab, grafana: siGrafana, hubspot: siHubspot,
  intercom: { ...siIntercom, hex: "1F8DED" }, jira: siJira, linear: siLinear, netlify: siNetlify,
  newrelic: siNewrelic, notion: siNotion, opsgenie: siOpsgenie, outline: siOutline, pagerduty: siPagerduty,
  plane: siPlane, postman: siPostman, render: siRender, sentry: siSentry, shortcut: siShortcut, snyk: siSnyk,
  sonarcloud: siSonarqubecloud, supabase: siSupabase, todoist: siTodoist, trello: siTrello, vercel: siVercel,
  zendesk: siZendesk, zotero: siZotero,
};

const IMAGES = { close, freshdesk, front, gong, gorgias, harvest, monday, pipedrive, raindrop, readwise, wrike, youtrack };

const SlackMark = ({ className }) => (
  <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
    <path fill="#E01E5A" d="M5.04 15.17a2.53 2.53 0 0 1-2.52 2.52A2.53 2.53 0 0 1 0 15.17a2.53 2.53 0 0 1 2.52-2.52h2.52v2.52zm1.27 0a2.53 2.53 0 0 1 2.52-2.52 2.53 2.53 0 0 1 2.52 2.52v6.31A2.53 2.53 0 0 1 8.83 24a2.53 2.53 0 0 1-2.52-2.52v-6.31z" />
    <path fill="#36C5F0" d="M8.83 5.04a2.53 2.53 0 0 1-2.52-2.52A2.53 2.53 0 0 1 8.83 0a2.53 2.53 0 0 1 2.52 2.52v2.52H8.83zm0 1.27a2.53 2.53 0 0 1 2.52 2.52 2.53 2.53 0 0 1-2.52 2.52H2.52A2.53 2.53 0 0 1 0 8.83a2.53 2.53 0 0 1 2.52-2.52h6.31z" />
    <path fill="#2EB67D" d="M18.96 8.83a2.53 2.53 0 0 1 2.52-2.52A2.53 2.53 0 0 1 24 8.83a2.53 2.53 0 0 1-2.52 2.52h-2.52V8.83zm-1.27 0a2.53 2.53 0 0 1-2.52 2.52 2.53 2.53 0 0 1-2.52-2.52V2.52A2.53 2.53 0 0 1 15.17 0a2.53 2.53 0 0 1 2.52 2.52v6.31z" />
    <path fill="#ECB22E" d="M15.17 18.96a2.53 2.53 0 0 1 2.52 2.52A2.53 2.53 0 0 1 15.17 24a2.53 2.53 0 0 1-2.52-2.52v-2.52h2.52zm0-1.27a2.53 2.53 0 0 1-2.52-2.52 2.53 2.53 0 0 1 2.52-2.52h6.31A2.53 2.53 0 0 1 24 15.17a2.53 2.53 0 0 1-2.52 2.52h-6.31z" />
  </svg>
);

// Near-black marks follow the text colour so they stay visible in dark mode.
const isDark = (hex) => {
  const n = parseInt(hex, 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b < 70;
};

/** A connector's logo in a square tile; initials when there is none. */
export default function BrandLogo({ id, name, size = "md" }) {
  const box = size === "sm" ? "size-9 rounded-lg" : "size-11 rounded-xl";
  const mark = size === "sm" ? "size-5" : "size-6";
  const icon = ICONS[id];
  const img = IMAGES[id];
  let inner;
  if (id === "slack") {
    inner = <SlackMark className={mark} />;
  } else if (icon) {
    inner = (
      <svg viewBox="0 0 24 24" className={`${mark} ${isDark(icon.hex) ? "text-ink" : ""}`} aria-hidden="true"
        fill={isDark(icon.hex) ? "currentColor" : `#${icon.hex}`}>
        <path d={icon.path} />
      </svg>
    );
  } else if (img) {
    inner = <img src={img} alt="" className={`${mark} rounded-[5px] object-contain`} loading="lazy" />;
  } else {
    inner = <span className="text-[12px] font-bold font-mono text-muted">{(name || id).slice(0, 2).toUpperCase()}</span>;
  }
  return (
    <div className={`${box} border border-line bg-surface flex items-center justify-center shrink-0`} title={name}>
      {inner}
    </div>
  );
}
