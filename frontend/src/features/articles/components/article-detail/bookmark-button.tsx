"use client";
import {useFormState} from "react-dom";
import {Tooltip, Box} from "@mantine/core";
import {IconBookmark, IconBookmarkFilled} from "@tabler/icons-react";
import {FormActionButton} from "@/components/form-action-button";
import {bookmark} from "../../actions/bookmark";
import classes from "./bookmark-button.module.css";

type Props = {
  uuid: string;
  title: string;
  isBookmarked: boolean;
};

export function BookmarkButton({uuid, title, isBookmarked}: Props) {
  const [state, dispatch] = useFormState(bookmark, {
    success: true,
    bookmarked: isBookmarked,
    errorMessage: "",
  });
  const bookmarked = state.bookmarked;

  return (
    <Box component="form" lh={0} action={dispatch}>
      <Tooltip
        label={bookmarked ? "حذف از بوکمارک ها" : "ذخیره کردن"}
        withArrow
      >
        <FormActionButton
          variant="transparent"
          c={"dimmed"}
          ml={-7}
          loadingPlaceholder={
            <IconBookmarkFilled className={classes.opacity50} />
          }
        >
          {bookmarked ? <IconBookmarkFilled /> : <IconBookmark size={50} />}
        </FormActionButton>
      </Tooltip>
      <input type="text" value={uuid} name="uuid" readOnly hidden />
      <input type="text" value={title} name="title" readOnly hidden />
    </Box>
  );
}
