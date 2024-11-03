import {Text, Avatar, Group} from "@mantine/core";

type Props = {
  avatar: string;
  name: string;
  message: string;
  date: string;
};

export function Comment({avatar, name, message, date}: Props) {
  return (
    <div>
      <Group>
        <Avatar src={avatar} alt={name} radius="xl" />
        <div>
          <Text size="sm">{name}</Text>
          <Text size="xs" c="dimmed">
            {date}
          </Text>
        </div>
      </Group>
      <Text pl={54} pt="sm" size="sm">
        {message}
      </Text>
    </div>
  );
}
