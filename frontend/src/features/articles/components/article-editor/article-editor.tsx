"use client";
import {useMemo, type RefObject} from "react";
import {CKEditor} from "@ckeditor/ckeditor5-react";
import {ClassicEditor, EditorConfig} from "ckeditor5";
import {editorConfig} from "./editor-config";
import "ckeditor5/ckeditor5.css";
import "./article-editor.css";

export type EditorRef = CKEditor<ClassicEditor>;

type Props = {
  initialData?: string;
  editorRef?: RefObject<EditorRef>;
};

export function ArticleEditor({initialData, editorRef}: Props) {
  const config: EditorConfig = useMemo(() => {
    return {
      ...editorConfig,
      initialData: initialData || "",
    };
  }, [initialData]);

  return (
    <div className="main-container">
      <div className="editor-container editor-container_classic-editor editor-container_include-style editor-container_include-block-toolbar editor-container_include-word-count">
        <div className="editor-container__editor">
          {config && (
            <CKEditor editor={ClassicEditor} config={config} ref={editorRef} />
          )}
        </div>
      </div>
    </div>
  );
}
