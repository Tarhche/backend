import {forwardRef} from "react";
import {useComputedColorScheme} from "@mantine/core";
import RichTextEditor, {Editor} from "reactjs-tiptap-editor";
import {extensions} from "./extensions";
import "katex/dist/katex.min.css";
import "reactjs-tiptap-editor/style.css";

export type Ref = {
  editor: Editor;
};

type Props = {
  initialContent?: string;
};

export const ArticleEditor = forwardRef<Ref, Props>((props, ref) => {
  const {initialContent = ""} = props;
  const theme = useComputedColorScheme();

  return (
    <div dir="ltr">
      <RichTextEditor
        ref={ref}
        output="html"
        content={initialContent}
        extensions={extensions}
        useEditorOptions={{
          immediatelyRender: false,
          shouldRerenderOnTransaction: false,
        }}
        dark={theme === "dark"}
      />
    </div>
  );
});

ArticleEditor.displayName = "ArticleRichTextEditor";
