import {
  BaseKit,
  Blockquote,
  Bold,
  BulletList,
  Clear,
  Code,
  CodeBlock,
  Color,
  ColumnActionButton,
  Emoji,
  FontSize,
  FormatPainter,
  Heading,
  Highlight,
  History,
  HorizontalRule,
  Iframe,
  Image,
  Indent,
  Italic,
  Katex,
  LineHeight,
  Link,
  MoreMark,
  OrderedList,
  SlashCommand,
  Strike,
  Table,
  TaskList,
  TextAlign,
  Underline,
  Video,
  Excalidraw,
  TextDirection,
  Mention,
} from "reactjs-tiptap-editor";
import {ImageUploadButton} from "./image-upload-button";

export const extensions = [
  BaseKit.configure({
    multiColumn: true,
    placeholder: {
      showOnlyCurrent: true,
    },
  }),
  History,
  TextDirection.configure({
    types: ["heading", "paragraph", "blockquote", "list_item"],
    directions: ["ltr", "rtl"],
    defaultDirection: "rtl",
  }),
  FormatPainter.configure({spacer: true}),
  Clear,
  Heading.configure({spacer: true}),
  FontSize,
  Bold,
  Italic,
  Underline,
  Strike,
  MoreMark,
  Katex.configure({
    HTMLAttributes: {
      dir: "ltr",
    },
  }),
  Emoji,
  Color.configure({spacer: true}),
  Highlight,
  BulletList.configure({
    HTMLAttributes: {
      dir: "rtl",
    },
  }),
  OrderedList.configure({
    HTMLAttributes: {
      dir: "rtl",
    },
  }),
  TextAlign.configure({
    types: ["heading", "paragraph"],
    spacer: true,
    defaultAlignment: "ltr",
  }),
  Indent,
  LineHeight,
  TaskList.configure({
    spacer: true,
    taskItem: {
      nested: true,
    },
  }),
  Link,
  Image.configure({
    button: ({editor, extension, t}) => {
      return {
        component: ImageUploadButton,
        componentProps: {
          action: () => {},
          upload: extension.options.upload,
          disabled: !editor.can().setImage,
          icon: "ImageUp",
          tooltip: t("editor.image.tooltip"),
          editor,
        },
      };
    },
  }).extend({
    parseHTML() {
      return [
        {
          tag: "img",
          getAttrs: (img) => {
            const element = img?.parentElement;

            const width = img?.getAttribute("width");

            const flipX = img?.getAttribute("flipx") || false;
            const flipY = img?.getAttribute("flipy") || false;

            return {
              src: img?.getAttribute("src"),
              alt: img?.getAttribute("alt"),
              caption: img?.getAttribute("caption"),
              width: width ? Number.parseInt(width, 10) : null,
              align:
                img?.getAttribute("align") || element?.style?.textAlign || null,
              inline: img?.getAttribute("inline") || false,
              flipX: flipX === "true",
              flipY: flipY === "true",
            };
          },
        },
        {
          tag: "span.image img",
          getAttrs: (img) => {
            const element = img?.parentElement;

            const width = img?.getAttribute("width");

            const flipX = img?.getAttribute("flipx") || false;
            const flipY = img?.getAttribute("flipy") || false;

            return {
              src: img?.getAttribute("src"),
              alt: img?.getAttribute("alt"),
              caption: img?.getAttribute("caption"),
              width: width ? Number.parseInt(width, 10) : null,
              align:
                img?.getAttribute("align") || element?.style?.textAlign || null,
              inline: img?.getAttribute("inline") || false,
              flipX: flipX === "true",
              flipY: flipY === "true",
            };
          },
        },
        {
          tag: "div[class=image]",
          getAttrs: (element) => {
            const img = element.querySelector("img");

            const width = img?.getAttribute("width");
            const flipX = img?.getAttribute("flipx") || false;
            const flipY = img?.getAttribute("flipy") || false;

            return {
              src: img?.getAttribute("src"),
              alt: img?.getAttribute("alt"),
              caption: img?.getAttribute("caption"),
              width: width ? Number.parseInt(width, 10) : null,
              align:
                img?.getAttribute("align") || element.style.textAlign || null,
              inline: img?.getAttribute("inline") || false,
              flipX: flipX === "true",
              flipY: flipY === "true",
            };
          },
        },
      ];
    },
  }),
  Video.configure({
    upload: (files: File) => {
      return new Promise((resolve) => {
        setTimeout(() => {
          resolve(URL.createObjectURL(files));
        }, 500);
      });
    },
  }),
  Blockquote.configure({spacer: true}),
  SlashCommand,
  HorizontalRule,
  Code.configure({
    toolbar: false,
  }),
  CodeBlock.configure({defaultTheme: "dracula"}),
  ColumnActionButton,
  Table,
  Iframe,
  Excalidraw,
  Mention,
];
