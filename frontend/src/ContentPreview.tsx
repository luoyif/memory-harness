import { ReactNode, useEffect } from "react";
import { ArrowRight, X } from "lucide-react";
import { ReadableMarkdown } from "./ReadableContent";

export function ContentPreview({
  title,
  content,
  meta,
  open,
  onOpen,
  onClose,
}: {
  title: string;
  content: string;
  meta?: ReactNode;
  open: boolean;
  onOpen: () => void;
  onClose: () => void;
}) {
  useEffect(() => {
    if (!open) return;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const needsDrawer = content.length > 240 || content.split("\n").length > 3;
  return (
    <>
      <div className="content-preview">
        <p>{content || "尚未填写内容"}</p>
        {needsDrawer && (
          <button type="button" className="text-button" onClick={onOpen}>
            查看全文 <ArrowRight size={14} />
          </button>
        )}
      </div>
      {open && (
        <div className="drawer-backdrop" role="presentation" onMouseDown={onClose}>
          <aside
            className="drawer content-preview-drawer"
            role="dialog"
            aria-modal="true"
            aria-labelledby="content-preview-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button className="drawer-close" type="button" onClick={onClose} aria-label="关闭全文">
              <X size={17} />
            </button>
            <p className="micro">FULL CONTENT</p>
            <h2 id="content-preview-title">{title}</h2>
            {meta && <div className="content-preview-meta">{meta}</div>}
            <ReadableMarkdown content={content || "尚未填写内容"} className="content-preview-full" />
          </aside>
        </div>
      )}
    </>
  );
}
