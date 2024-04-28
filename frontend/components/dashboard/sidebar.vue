<template>
	<aside class="flex-shrink-0">
		<ul class="list-unstyled px-0">
			<li class="my-2">
				<NuxtLink to="/dashboard" class="btn align-items-center rounded collapsed">
					<span class="bi ms-2 fa fa-dashboard"></span>
					<span>داشبورد</span>
				</NuxtLink>
			</li>
			<li class="my-2">
				<NuxtLink to="/dashboard/articles" class="btn align-items-center rounded collapsed">
					<span class="bi ms-2 fa fa-pen"></span>
					<span>مقاله ها</span>
				</NuxtLink>
			</li>
			<li class="my-2">
				<NuxtLink to="/dashboard/files" class="btn align-items-center rounded collapsed">
					<span class="bi ms-2 fa fa-file"></span>
					<span>فایل ها</span>
				</NuxtLink>
			</li>
			<li class="border-top my-3"></li>
			<li class="my-2">
				<NuxtLink to="/dashboard" class="btn align-items-center rounded collapsed">
					<span class="bi ms-2 fa fa-user"></span>
					<span>پروفایل</span>
				</NuxtLink>
			</li>
		</ul>
	</aside>
</template>
