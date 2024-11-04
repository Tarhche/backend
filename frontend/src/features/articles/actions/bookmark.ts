"use server";
import * as z from "zod";
import {bookmarkArticle} from "@/dal/bookmarks";

type FormState = {
  success: boolean;
  bookmarked: boolean;
  errorMessage?: string;
};

const SCHEMA = z.object({
  title: z.string(),
  uuid: z.string().uuid(),
});

export async function bookmark(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const data: Record<string, any> = {};
  formData.forEach((value, key) => {
    data[key] = value;
  });
  const isBookmarked = formState.bookmarked;
  const validatedData = await SCHEMA.safeParseAsync(data);

  try {
    if (validatedData.success === false) {
      throw new Error();
    }
    await bookmarkArticle({
      keep: !isBookmarked,
      uuid: validatedData.data.uuid,
      title: validatedData.data.title,
    });
    return {
      success: true,
      bookmarked: !isBookmarked,
    };
  } catch {
    return {
      success: false,
      bookmarked: isBookmarked,
    };
  }
}
