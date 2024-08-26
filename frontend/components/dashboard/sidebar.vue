<template>
  <aside class="flex-shrink-0">
    <ul class="list-unstyled px-0">
      <li class="my-2">
        <NuxtLink to="/dashboard" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-dashboard"></span>
          <span>داشبورد</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('articles.index')" class="my-2">
        <NuxtLink to="/dashboard/articles" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-pen"></span>
          <span>مقاله ها</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('comments.index')" class="my-2">
        <NuxtLink to="/dashboard/comments" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-comments"></span>
          <span>کامنت ها</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('files.index')" class="my-2">
        <NuxtLink to="/dashboard/files" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-file"></span>
          <span>فایل ها</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('self.comments.index') || hasPermission('self.bookmarks.index')" class="border-top my-3"></li>
      <li v-if="hasPermission('self.comments.index')" class="my-2">
        <NuxtLink to="/dashboard/my/comments" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa-regular fa-comments"></span>
          <span>کامنت های من</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('self.bookmarks.index')" class="my-2">
        <NuxtLink to="/dashboard/my/bookmarks" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa-regular fa-bookmark"></span>
          <span>بوک مارک های من</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('users.index') || hasPermission('roles.index') || hasPermission('config.show')" class="border-top my-3"></li>
      <li v-if="hasPermission('users.index')" class="my-2">
        <NuxtLink to="/dashboard/users" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-user"></span>
          <span>کاربرها</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('roles.index')" class="my-2">
        <NuxtLink to="/dashboard/roles" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-shield-halved"></span>
          <span>نقش ها</span>
        </NuxtLink>
      </li>
      <li v-if="hasPermission('config.show')" class="my-2">
        <NuxtLink to="/dashboard/config" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa-solid fa-gear"></span>
          <span>تنظیمات</span>
        </NuxtLink>
      </li>
      <li class="border-top my-3"></li>
      <li class="my-2">
        <NuxtLink to="/dashboard/profile" class="btn align-items-center rounded collapsed">
          <span class="bi ms-2 fa fa-user"></span>
          <span>پروفایل</span>
        </NuxtLink>
      </li>
    </ul>
  </aside>
</template>

<script setup>
const permissions = await useUser().permissions()

function hasPermission(permission) {
  return permissions.includes(permission)
}
</script>