"use client";
import {useRef, useMemo} from "react";
import {CKEditor} from "@ckeditor/ckeditor5-react";
import {ClassicEditor, EditorConfig} from "ckeditor5";
import {editorConfig} from "./editor-config";
import "ckeditor5/ckeditor5.css";
import "./article-editor.css";

type Props = {
  initialData?: string;
};

export function ArticleEditor({initialData}: Props) {
  const editorContainerRef = useRef<any>(null);
  const editorRef = useRef<any>(null);
  const editorWordCountRef = useRef<any>(null);

  const config: EditorConfig = useMemo(() => {
    return {
      ...editorConfig,
      initialData: initialData || "",
    };
  }, [initialData]);

  return (
    <div className="main-container">
      <div
        className="editor-container editor-container_classic-editor editor-container_include-style editor-container_include-block-toolbar editor-container_include-word-count"
        ref={editorContainerRef}
      >
        <div className="editor-container__editor">
          <div ref={editorRef}>
            {config && <CKEditor editor={ClassicEditor} config={config} />}
          </div>
        </div>
        <div
          className="editor_container__word-count"
          ref={editorWordCountRef}
        ></div>
      </div>
    </div>
  );
}
