"use client";
import Link from "next/link";
import {useFormState} from "react-dom";
import {
  TextInput,
  PasswordInput,
  Text,
  Paper,
  Group,
  Container,
  Stack,
  Alert,
  Anchor,
  List,
  ListItem,
} from "@mantine/core";
import {IconInfoCircle} from "@tabler/icons-react";
import {FormButton} from "@/components/form-button";
import {FieldErrors} from "./field-errors";
import {verifyUser} from "../actions/verify-user";

type Props = {
  token: string;
};

export function VerifyForm({token}: Props) {
  const [state, dispatch] = useFormState(verifyUser, {
    success: false,
    nonFieldErrors: [],
  });
  const fieldErrors = state.fieldErrors;

  if (state.success) {
    return (
      <Container size={500} p={0} mt={"xl"}>
        <Paper radius={"md"} p={"xl"} withBorder>
          <Alert
            variant="filled"
            color="green"
            title="ثبت نام موفق"
            mt={"sm"}
            icon={<IconInfoCircle />}
          >
            حساب شما با موفقیت تایید شد، لطفا به صفحه{" "}
            <Anchor
              c={"white"}
              underline="always"
              component={Link}
              href={"/auth/login"}
            >
              ورود
            </Anchor>{" "}
            مراجعه کنید و وارد حساب خود شوید
          </Alert>
        </Paper>
      </Container>
    );
  }

  return (
    <Container size={500} p={0} mt={"xl"}>
      <Paper radius="md" p="xl" withBorder>
        <Text size="lg" mb={"lg"} fw={500}>
          جهت تکمیل ثبت نام اطلاعات زیر را وارد کنید
        </Text>
        <form action={dispatch}>
          <Stack>
            <Stack gap={8}>
              <TextInput
                name="name"
                label="نام"
                radius="md"
                error={Boolean(fieldErrors?.name?.length)}
                required
              />
              <FieldErrors errors={fieldErrors?.name} />
            </Stack>
            <Stack gap={8}>
              <TextInput
                name="username"
                label="نام کاربری (یوزرنیم)"
                radius="md"
                error={Boolean(fieldErrors?.username?.length)}
                required
              />
              <FieldErrors errors={fieldErrors?.username} />
            </Stack>
            <Stack gap={8}>
              <PasswordInput
                name="password"
                label="کلمه عبور"
                radius="md"
                error={Boolean(fieldErrors?.password?.length)}
                required
              />
              <FieldErrors errors={fieldErrors?.password} />
            </Stack>
            <Stack gap={8}>
              <PasswordInput
                name="repassword"
                label="تکرار کلمه عبور"
                radius="md"
                error={Boolean(fieldErrors?.repassword?.length)}
                required
              />
              <FieldErrors errors={fieldErrors?.repassword} />
            </Stack>
            <input name="token" value={token} hidden readOnly />
          </Stack>
          {state.nonFieldErrors?.map((err) => {
            return (
              <Alert
                key={err}
                variant="filled"
                color="red"
                title="ثبت نام ناموفق"
                mt={"sm"}
                icon={<IconInfoCircle />}
              >
                <List>
                  {state.nonFieldErrors?.map((err) => {
                    return (
                      <ListItem key={err} fz={"xs"}>
                        {err}
                      </ListItem>
                    );
                  })}
                </List>
              </Alert>
            );
          })}
          <Group
            justify="space-between"
            mt={(state?.nonFieldErrors?.length ?? 0) >= 1 ? "sm" : "xl"}
          >
            <FormButton type="submit" fullWidth>
              تکمیل ثبت نام
            </FormButton>
          </Group>
        </form>
      </Paper>
    </Container>
  );
}
