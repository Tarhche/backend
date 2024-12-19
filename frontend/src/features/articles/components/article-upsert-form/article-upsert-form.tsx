"use client";
import {useRef} from "react";
import {useFormState} from "react-dom";
import {
  Box,
  Group,
  Stack,
  Textarea,
  TextInput,
  InputLabel,
  TagsInput,
} from "@mantine/core";
import {DateTimeInput} from "@/components/date-time-input";
import {FormButton} from "@/components/form-button";
import {
  ArticleEditor,
  type Ref,
} from "@/features/articles/components/article-editor";
import {FileInput} from "./file-input";
import {IconPhotoPlus, IconMovie} from "@tabler/icons-react";
import {upsertArticleAction} from "../../actions/upsert-article";
import {isGregorianStartDateTime} from "@/lib/date-and-time";

type Props = {
  article?: {
    articleId: string;
    defaultTitle: string;
    defaultExcerpt: string;
    defaultBody: string;
    defaultHashtags: string[];
    defaultCover: string;
    defaultVideo: string;
    defaultPublishedAt: string;
  };
};

export function ArticleUpsertForm({article}: Props) {
  const editorRef = useRef<Ref>(null);
  const [state, dispatch] = useFormState(upsertArticleAction, {
    success: true,
  });
  const defaultPublishedDate = article?.defaultPublishedAt
    ? isGregorianStartDateTime(article.defaultPublishedAt)
      ? null
      : new Date(article.defaultPublishedAt)
    : null;

  const handleSubmit = async (formData: FormData) => {
    formData.set("body", editorRef.current?.editor.getHTML() || "");
    if (article?.articleId) {
      formData.set("uuid", article.articleId);
    }
    dispatch(formData);
  };

  return (
    <form action={handleSubmit}>
      <Stack gap="lg">
        <TextInput
          name="title"
          label="عنوان مقاله"
          defaultValue={article?.defaultTitle ?? ""}
          error={state.fieldErrors?.title ?? ""}
        />
        <Textarea
          name="excerpt"
          label="خلاصه محتوا"
          defaultValue={article?.defaultExcerpt ?? ""}
          error={state.fieldErrors?.excerpt ?? ""}
          autosize
        />
        <Box>
          <InputLabel>محتوا</InputLabel>
          <ArticleEditor
            ref={editorRef}
            initialContent={article?.defaultBody || ""}
          />
        </Box>
        <FileInput
          name="cover"
          label="کاور"
          defaultValue={article?.defaultCover || ""}
          icon={<IconPhotoPlus size={50} />}
        />
        <FileInput
          name="video"
          label="ویدئو"
          defaultValue={article?.defaultVideo || ""}
          icon={<IconMovie size={50} />}
        />
        <TagsInput
          name="tags"
          label="تگ ها"
          splitChars={[" "]}
          defaultValue={article?.defaultHashtags || []}
          clearable
        />
        <DateTimeInput
          name="published_at"
          label="تاریخ انتشار"
          defaultValue={defaultPublishedDate}
          clearable
        />
        <Group justify="flex-end" mt="lg">
          <FormButton>
            {article?.articleId ? "بروزرسانی مقاله" : "ایجاد مقاله"}
          </FormButton>
        </Group>
      </Stack>
    </form>
  );
}
