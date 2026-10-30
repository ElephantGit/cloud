/**
 * The browser's answers to the desktop platform calls the editor makes. Desktop asks Tauri for a
 * save dialog and listens for OS file drops; a page downloads the file instead, and dropped
 * files already arrive through HTML5 `dataTransfer.files`, so the drop hooks stay absent.
 */
export interface SaveTextFileOptions {
  defaultFileName: string;
  content: string;
}

export interface OsDroppedTextFile {
  name: string;
  size: number;
  content: string;
}

export interface WorkflowPlatform {
  saveTextFile(options: SaveTextFileOptions): Promise<boolean>;
  openExternalUrl(url: string): Promise<void>;
  subscribeOsFileDrop?(
    listener: (paths: readonly string[]) => void,
  ): () => void;
  readOsTextFile?(path: string): Promise<OsDroppedTextFile>;
}

const browserPlatform: WorkflowPlatform = {
  async saveTextFile({ defaultFileName, content }) {
    const url = URL.createObjectURL(
      new Blob([content], { type: "application/json" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = defaultFileName;
    link.click();
    URL.revokeObjectURL(url);
    return true;
  },
  async openExternalUrl(url) {
    window.open(url, "_blank", "noopener,noreferrer");
  },
};

export function usePlatform(): WorkflowPlatform {
  return browserPlatform;
}

export function useOptionalPlatform(): WorkflowPlatform | null {
  return browserPlatform;
}
