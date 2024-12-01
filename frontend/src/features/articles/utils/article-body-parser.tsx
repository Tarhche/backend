import parse, {Element, domToReact} from "html-react-parser";
import Image from "next/image";
import {ImageZoom} from "@/components/image-zoom";
import {CodeHighlight} from "@mantine/code-highlight";
import "@mantine/code-highlight/styles.css";

export function parseArticleBodyToReact(html: string) {
  return parse(html, {
    replace(domNode) {
      if (domNode instanceof Element && domNode.name === "pre") {
        const codeElement = domNode.children.find((child) => {
          if (child instanceof Element && child.name === "code") {
            return true;
          }
          return false;
        });

        if (codeElement instanceof Element && codeElement.childNodes) {
          // @ts-expect-error I think because childNodes comes from react-html-parse this is okay to do
          const codeContent = domToReact(codeElement.childNodes).toString();
          const language = codeElement.attribs.class.replace("language-", "");

          return (
            <CodeHighlight
              mt="sm"
              mb="xl"
              code={codeContent}
              language={language.trim()}
              copyLabel="کپی کردن"
              copiedLabel="کپی شد!"
              styles={{
                code: {
                  fontSize: 14,
                },
              }}
            />
          );
        }

        return null;
      } else if (domNode instanceof Element && domNode.name === "img") {
        const {src, alt} = domNode.attribs;

        return (
          <ImageZoom>
            <Image
              width={1200}
              height={720}
              alt={alt || "article figures"}
              src={src}
            />
          </ImageZoom>
        );
      }
    },
  });
}
