"use client";
import {useRef} from "react";
import {useFormState} from "react-dom";
import {Stack, Group, Text, Textarea} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {AuthUserAvatar} from "@/components/auth-user-avatar";
import {
  IconSend,
  IconExclamationCircle,
  IconCircleDashedCheck,
} from "@tabler/icons-react";
import clsx from "clsx";
import {comment} from "../../actions/comment";
import classes from "./comment-form.module.css";

type Props = {
  objectUUID: string;
  parentUUID: string;
};

export function CommentForm({objectUUID, parentUUID}: Props) {
  const formRef = useRef<HTMLFormElement>(null);
  const [state, dispatch] = useFormState(comment, {});
  const isSuccessful = state.success;
  const isReplying = Boolean(parentUUID);

  if (isSuccessful) {
    formRef.current?.reset();
  }

  return (
    <form ref={formRef} action={dispatch}>
      <Stack gap={"xs"}>
        <Group align="start" gap={10}>
          <AuthUserAvatar />
          <Stack flex={1} gap={10}>
            <Textarea
              placeholder={
                isReplying
                  ? "پاسخ خود را بنویسید"
                  : "دیدگاه خود را اینجا بنویسید"
              }
              rows={4}
              name="body"
              classNames={{
                input: clsx({[classes.redBorder]: isSuccessful === false}),
              }}
            />
            {isSuccessful && (
              <Text
                className={clsx(classes.text, classes.successText)}
                size="sm"
              >
                <IconCircleDashedCheck size={20} />
                دیدگاه شما با موفقیت ثبت گردید. پس از بازبینی منتشر خواهد شد
              </Text>
            )}
            {isSuccessful === false && (
              <Text className={clsx(classes.text, classes.errorText)} size="sm">
                <IconExclamationCircle size={20} />
                {state.errorMessage
                  ? state.errorMessage
                  : `متاسفانه در پردازش دیدگاه شما خطایی بوجود آمد. لطفا مجددا تلاش
                نمایید`}
              </Text>
            )}
          </Stack>
        </Group>
        <input name="object-uuid" value={objectUUID} hidden readOnly />
        <input name="parent-uuid" value={parentUUID} hidden readOnly />
        <FormButton
          leftSection={<IconSend size={20} />}
          style={{
            alignSelf: "flex-end",
          }}
        >
          ارسال دیدگاه
        </FormButton>
      </Stack>
    </form>
  );
}
