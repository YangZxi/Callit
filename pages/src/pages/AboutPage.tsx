import { title } from "@/components/primitives";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { currentLocale, chineseLocale } from "@/i18n";

async function loadMarked(readmeName: string) {
  // const styleUrl = `https://cdn.jsdelivr.net/npm/github-markdown-css@5/github-markdown-${theme}.min.css`;
  const styleUrl = `https://cdn.jsdelivr.net/npm/github-markdown-css@5/github-markdown.min.css`;
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = styleUrl;
  document.head.appendChild(link);
  // @ts-ignore
  const { marked } = await import("https://cdn.jsdelivr.net/npm/marked/lib/marked.esm.js");
  const mdUrl = `https://raw.githubusercontent.com/YangZxi/Callit/refs/heads/main/${readmeName}`;
  const resp = await fetch(mdUrl);
  let mdText = await resp.text();
  const imgPrefix = "https://github.com/YangZxi/Callit/raw/main/";
  // @ts-ignore
  mdText = mdText.replaceAll("](./docs/images/", `](${imgPrefix}docs/images/`);
  document.getElementById('content')!.innerHTML =
    marked.parse(mdText);
}

export default function AboutPage() {
  const { t, i18n } = useTranslation();

  useEffect(() => {
    void loadMarked(currentLocale() === chineseLocale ? "README.md" : "README_EN.md");
  }, [i18n.language]);

  return (
    <section className="flex flex-col items-center justify-center gap-4 py-2 md:py-4">
      <div className="inline-block max-w-[720px] text-center justify-center">
        <h1 className={title()}>{t("about.title")}</h1>
        <div id="content" className="markdown-body mt-3 text-left"></div>
      </div>
    </section>
  );
}
