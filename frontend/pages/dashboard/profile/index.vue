<template>
	<div class="container">
		<div class="row">
			<dashboardSidebar class="col-md-3 ml-sm-auto"/>
			<main class="col-md-9 ml-sm-auto">

				<nav aria-label="breadcrumb">
					<ol class="breadcrumb">
						<li class="breadcrumb-item">
							<NuxtLink to="/dashboard">داشبورد</NuxtLink>
						</li>
						<li class="breadcrumb-item active" aria-current="page">پروفایل</li>
					</ol>
				</nav>

                <form @submit.prevent="updateProfile()" class="card">
                    <div class="card-body">
                        <div class="row mb-3">
                            <label for="name" class="col-sm-2 col-form-label">نام</label>
                            <div class="col-sm-10">
                            <input type="text" placeholder="نام" class="form-control" id="name" v-model="params.name">
                            </div>
                        </div>

                        <div class="row mb-3">
                            <label for="email" class="col-sm-2 col-form-label">ایمیل</label>
                            <div class="col-sm-10">
                            <input type="email" placeholder="ایمیل" class="form-control" id="email" v-model="params.email" required>
                            </div>
                        </div>

                        <div class="row mb-3">
                            <label for="username" class="col-sm-2 col-form-label">یوزرنیم</label>
                            <div class="col-sm-10">
                            <input type="text" placeholder="یوزرنیم" class="form-control" id="username" v-model="params.username">
                            </div>
                        </div>

                        <div class="mb-3">
                            <div @click.prevent="params.showFilePicker=true" class="image-picker" :style="{ backgroundImage: `url('${ useFilesUrlResolver().resolve(params.avatar) }')` }">
                                <small class="title">تصویر کاربر</small>
                                <div class="body">
                                    <small class="fa fa-plus"></small>
                                </div>
                            </div>
                        </div>

                        <p class="alert alert-secondary">
                            <span>کلمه عبور خود را میتوانید از</span>
                            <NuxtLink class="mx-1" to="/dashboard/profile/password">اینجا</NuxtLink>
                            <span>تغییر دهید</span>
                        </p>
                    </div>
                    <div class="card-footer">
                        <button :disabled="params.loading" type="submit" class="btn btn-primary rounded submit px-3">
                            <span v-if="!params.loading">ذخیره کن</span>
                            <div v-else class="spinner-border" role="status">
                                <span class="visually-hidden">Loading...</span>
                            </div>
                        </button>
                    </div>
                </form>

				<dashboardFileManager modal selectable :show="params.showFilePicker" @close="params.showFilePicker=false" @select="selectFile"/>

			</main>
		</div>
	</div>
</template>

<script lang="ts" setup>
definePageMeta({
	layout: 'dashboard',
})

useHead({
	title: "پروفایل"
})

// reflects form parameters
const params = reactive({
    email: '', // email is required
    name: null,
    username: null,
    avatar: null,
    loading: false,
	showFilePicker: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
	email: null,
	name: null,
	username: null,
	avatar: null,
})

await showProfile()

function selectFile(uuids:string[]) {
	params.showFilePicker = false

	if (params.avatar && params.avatar.length == 0) {
		return
	}

	params.avatar = uuids[0]
}

async function showProfile() {
    try {
        const data = await useUser().profile()

        params.name = data.name
        params.email = data.email
        params.username = data.username
        params.avatar = data.avatar
    } catch(error) {
        console.log(error)
    }
}

async function updateProfile() {
	params.loading = true

	try {
		await useUser().updateProfile(
            params.email,
            params.name,
            params.username,
            params.avatar,
		)
	} catch (error) {
		console.log(error)
	}

	params.loading = false
}
</script>
