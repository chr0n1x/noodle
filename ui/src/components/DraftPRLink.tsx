function prLabel(url: string) {
  const match = /\/pull\/(\d+)/.exec(url);
  return match ? `Draft PR #${match[1]}` : "Draft PR";
}

export function DraftPRLink({ url, branch }: { url: string; branch?: string }) {
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="block pl-9 pb-1 font-mono text-[10px] text-green underline"
      title={branch}
    >
      {prLabel(url)}
    </a>
  );
}
