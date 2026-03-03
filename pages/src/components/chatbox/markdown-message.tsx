"use client";

import React from "react";

let markedLoader: Promise<{marked: {parse: (source: string) => string}}> | null = null;

async function loadMarked() {
  if (!markedLoader) {
    // 与 AboutPage.tsx 保持一致，使用 marked 作为 Markdown 渲染库
    // @ts-ignore
    markedLoader = import("https://cdn.jsdelivr.net/npm/marked/lib/marked.esm.js");
  }

  return markedLoader;
}

export default function MarkdownMessage({content}: {content: string}) {
  const [html, setHtml] = React.useState("");

  React.useEffect(() => {
    let active = true;

    const renderMarkdown = async () => {
      try {
        const {marked} = await loadMarked();
        const rendered = marked.parse(content);

        if (active) {
          setHtml(rendered);
        }
      } catch {
        if (active) {
          setHtml("");
        }
      }
    };

    void renderMarkdown();

    return () => {
      active = false;
    };
  }, [content]);

  if (!html) {
    return <p className="whitespace-pre-wrap">{content}</p>;
  }

  return (
    <div
      className="space-y-2 [&_ol]:list-decimal [&_ol]:space-y-1 [&_ol]:pl-5 [&_p]:mb-3 [&_strong]:font-semibold"
      dangerouslySetInnerHTML={{__html: html}}
    />
  );
}
