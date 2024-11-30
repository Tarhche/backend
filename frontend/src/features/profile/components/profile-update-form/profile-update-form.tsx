"use client";
import Link from "next/link";
import {useEffect} from "react";
import {useFormState} from "react-dom";
import {Group, Stack, TextInput, Anchor, Alert} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {UserAvatarInput} from "@/components/user-avatar-input";
import {APP_PATHS} from "@/lib/app-paths";
import {notifications} from "@mantine/notifications";
import {updateProfileAction} from "../../actions/update-profile";

type Props = {
  userInfo: {
    name: string;
    email: string;
    username: string;
    avatar: string;
  };
};

export function ProfileUpdateForm({userInfo}: Props) {
  const [state, dispatch] = useFormState(updateProfileAction, {
    success: null,
  });
  const {username, name, avatar, email} = userInfo;

  useEffect(() => {
    if (state.success) {
      notifications.show({
        title: "بروزرسانی موفق",
        message: "پروفایل با موفقیت بروز شد",
        color: "green",
      });
    }
  }, [state, state.success]);

  return (
    <form action={dispatch}>
      <Group align="flex-start" justify="center">
        <UserAvatarInput username={email} defaultValue={avatar} />
        <Stack flex={"1 1 300px"}>
          <TextInput
            type="text"
            name="name"
            defaultValue={name}
            error={state.fieldErrors?.name || ""}
          />
          <TextInput
            type="email"
            name="email"
            defaultValue={email}
            error={state.fieldErrors?.email || ""}
          />
          <TextInput
            name="username"
            type="text"
            defaultValue={username}
            error={state.fieldErrors?.username || ""}
          />
          <Alert>
            برای تغییر کلمه عبور از{" "}
            <Anchor
              component={Link}
              href={APP_PATHS.dashboard.profile.editPassword}
            >
              اینجا
            </Anchor>{" "}
            اقدام کنید
          </Alert>
          <Group justify="flex-end" mt="md">
            <FormButton>ویرایش پروفایل</FormButton>
          </Group>
        </Stack>
      </Group>
    </form>
  );
}
