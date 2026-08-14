import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useTranslation } from "react-i18next";

type WorkerFileTreeProps = {
  entries: string[];
  selectedFile: string;
  mainFilename: string;
  revealPath?: string;
  disabled?: boolean;
  onSelectFile: (filename: string) => void;
  onRequestUpload: (directory: string) => void;
  onRequestCreateFile: (directory: string) => void;
  onRequestDelete: (entry: string) => void;
};

type TreeNode = {
  name: string;
  path: string;
  isDirectory: boolean;
  children: TreeNode[];
};

type ContextMenuState = {
  entry: string;
  x: number;
  y: number;
};

function buildTree(entries: string[], mainFilename: string): TreeNode[] {
  const roots: TreeNode[] = [];
  const directoryByPath = new Map<string, TreeNode>();

  const ensureDirectory = (segments: string[]): TreeNode => {
    const directoryPath = `${segments.join("/")}/`;
    const existing = directoryByPath.get(directoryPath);
    if (existing) return existing;

    const node: TreeNode = {
      name: segments[segments.length - 1],
      path: directoryPath,
      isDirectory: true,
      children: [],
    };
    directoryByPath.set(directoryPath, node);
    if (segments.length === 1) {
      roots.push(node);
    } else {
      ensureDirectory(segments.slice(0, -1)).children.push(node);
    }
    return node;
  };

  for (const entry of entries) {
    const isDirectory = entry.endsWith("/");
    const normalized = isDirectory ? entry.slice(0, -1) : entry;
    const segments = normalized.split("/").filter(Boolean);
    if (segments.length === 0) continue;

    if (isDirectory) {
      ensureDirectory(segments);
      continue;
    }
    const node: TreeNode = {
      name: segments[segments.length - 1],
      path: entry,
      isDirectory: false,
      children: [],
    };
    if (segments.length === 1) {
      roots.push(node);
    } else {
      ensureDirectory(segments.slice(0, -1)).children.push(node);
    }
  }

  const sortNodes = (nodes: TreeNode[]) => {
    nodes.sort((left, right) => {
      if (left.isDirectory !== right.isDirectory) return left.isDirectory ? -1 : 1;
      if (!left.isDirectory) {
        if (left.path === mainFilename) return -1;
        if (right.path === mainFilename) return 1;
      }
      return left.name.localeCompare(right.name);
    });
    nodes.forEach((node) => sortNodes(node.children));
  };
  sortNodes(roots);
  return roots;
}

function directoryPathsFor(entry: string): string[] {
  const normalized = entry.endsWith("/") ? entry.slice(0, -1) : entry;
  const segments = normalized.split("/").filter(Boolean);
  const directoryCount = entry.endsWith("/") ? segments.length : Math.max(0, segments.length - 1);
  return Array.from({ length: directoryCount }, (_, index) => `${segments.slice(0, index + 1).join("/")}/`);
}

export default function WorkerFileTree({
  entries,
  selectedFile,
  mainFilename,
  revealPath,
  disabled = false,
  onSelectFile,
  onRequestUpload,
  onRequestCreateFile,
  onRequestDelete,
}: WorkerFileTreeProps) {
  const { t } = useTranslation();
  const tree = useMemo(() => buildTree(entries, mainFilename), [entries, mainFilename]);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const target = revealPath || selectedFile;
    if (!target) return;
    const paths = directoryPathsFor(target);
    if (paths.length === 0) return;
    setExpandedPaths((current) => new Set([...current, ...paths]));
  }, [revealPath, selectedFile]);

  useEffect(() => {
    if (!contextMenu) return;
    const closeOnPointerDown = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setContextMenu(null);
    };
    const close = () => setContextMenu(null);
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("pointerdown", closeOnPointerDown);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    return () => {
      document.removeEventListener("pointerdown", closeOnPointerDown);
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [contextMenu]);

  const toggleDirectory = (path: string) => {
    setExpandedPaths((current) => {
      const next = new Set(current);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const openContextMenu = (entry: string, x: number, y: number) => {
    const menuWidth = 144;
    const menuHeight = entry.endsWith("/") ? 128 : 48;
    const margin = 8;
    setContextMenu({
      entry,
      x: Math.max(margin, Math.min(x, window.innerWidth - menuWidth - margin)),
      y: Math.max(margin, Math.min(y, window.innerHeight - menuHeight - margin)),
    });
  };

  const renderNodes = (nodes: TreeNode[], depth: number) => nodes.map((node) => {
    const isExpanded = node.isDirectory && expandedPaths.has(node.path);
    const isSelected = !node.isDirectory && selectedFile === node.path;
    const isContextTarget = contextMenu?.entry === node.path;
    const isMainFile = !node.isDirectory && node.path === mainFilename;
    return (
      <div key={node.path} role="none">
        <button
          type="button"
          role="treeitem"
          aria-expanded={node.isDirectory ? isExpanded : undefined}
          aria-selected={isSelected}
          className={`flex h-8 w-full cursor-pointer items-center gap-1.5 rounded-sm pr-2 text-left text-sm outline-none transition-colors hover:bg-default focus-visible:ring-2 focus-visible:ring-primary ${
            isSelected ? "bg-default text-default-foreground hover:bg-default" : isContextTarget ? "bg-default text-default-foreground" : "text-default-700"
          }`}
          style={{ paddingLeft: `${6 + depth * 14}px` }}
          onClick={() => {
            if (node.isDirectory) toggleDirectory(node.path);
            else onSelectFile(node.path);
          }}
          onContextMenu={(event) => {
            event.preventDefault();
            openContextMenu(node.path, event.clientX, event.clientY);
          }}
          onKeyDown={(event) => {
            if (event.shiftKey && event.key === "F10") {
              event.preventDefault();
              const rect = event.currentTarget.getBoundingClientRect();
              openContextMenu(node.path, rect.left + 24, rect.bottom);
            }
          }}
          title={node.path}
        >
          <Icon
            aria-hidden="true"
            className={`size-4 shrink-0 text-default-500 transition-transform ${node.isDirectory && isExpanded ? "rotate-90" : ""}`}
            icon={node.isDirectory
              ? "material-symbols:chevron-right-rounded"
              : "material-symbols:description-outline-rounded"}
          />
          <span className="min-w-0 flex-1 truncate">{node.name}</span>
        </button>
        {node.isDirectory && isExpanded ? (
          <div role="group">{renderNodes(node.children, depth + 1)}</div>
        ) : null}
        {contextMenu?.entry === node.path ? (
          <div
            ref={menuRef}
            role="menu"
            aria-label={t("workers.fileMenu", { name: node.name })}
            className="fixed z-50 w-36 rounded-lg border border-default-200 bg-background p-1 shadow-lg"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            {node.isDirectory ? (
              <>
                <Button
                  size="sm"
                  variant="ghost"
                  className="w-full justify-start"
                  isDisabled={disabled}
                  onPress={() => {
                    setContextMenu(null);
                    onRequestUpload(node.path);
                  }}
                >
                  <Icon aria-hidden="true" className="size-4" icon="material-symbols:upload-file-outline-rounded" />
                  {t("workers.uploadFile")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="w-full justify-start"
                  isDisabled={disabled}
                  onPress={() => {
                    setContextMenu(null);
                    onRequestCreateFile(node.path);
                  }}
                >
                  <Icon aria-hidden="true" className="size-4" icon="material-symbols:note-add-outline-rounded" />
                  {t("workers.createFile")}
                </Button>
                <div aria-hidden="true" className="my-1 border-t border-default-200" />
              </>
            ) : null}
            <Button
              size="sm"
              variant="ghost"
              className="w-full justify-start text-danger"
              isDisabled={disabled || isMainFile}
              onPress={() => {
                setContextMenu(null);
                onRequestDelete(node.path);
              }}
            >
              <Icon aria-hidden="true" className="size-4" icon="material-symbols:delete-outline-rounded" />
              {t("common.delete")}
            </Button>
          </div>
        ) : null}
      </div>
    );
  });

  return (
    <div role="tree" aria-label={t("workers.title")} className="min-w-full p-1">
      {renderNodes(tree, 0)}
    </div>
  );
}
